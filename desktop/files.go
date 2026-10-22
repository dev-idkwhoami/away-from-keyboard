package main

import (
	"afk/internal/engine"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// File imports are exclusive with recording and model preparation.
func (s *Studio) ChooseFile() error {
	if s.app == nil {
		return errors.New("file picker requires the desktop app")
	}
	s.OpenStudio()
	path, err := s.app.Dialog.OpenFile().SetTitle("Transcribe audio file").
		AddFilter("Audio", "*.wav;*.mp3;*.m4a;*.flac;*.ogg;*.opus;*.aac;*.wma").
		AddFilter("All files", "*").PromptForSingleSelection()
	if err != nil || path == "" {
		return err
	}
	return s.TranscribeFile(path)
}
func (s *Studio) TranscribeFile(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("choose a regular audio file")
	}
	if _, err = exec.LookPath("ffmpeg"); err != nil {
		return errors.New("install ffmpeg to import audio files")
	}
	s.mu.Lock()
	if s.closing || s.preparing != "" || (s.status != "idle" && s.status != "error") {
		s.mu.Unlock()
		return errors.New("finish the current operation first")
	}
	config := s.saved.Settings
	if config.Diarization && !engine.DiarizationReady(s.root) {
		s.mu.Unlock()
		return errors.New("prepare Nemotron in Model Garage before identifying speakers")
	}
	installed := false
	for _, m := range engine.Catalogue(s.root) {
		if m.ID == config.Model && m.Installed {
			installed = true
		}
	}
	if !installed {
		s.mu.Unlock()
		return errors.New("download the selected model first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.status = "transcribing"
	s.fileName = filepath.Base(absolute)
	s.meetingFolder = ""
	s.fileStage = "Converting audio"
	s.started = time.Now()
	s.transcript, s.problem = "", ""
	s.turns = nil
	s.mu.Unlock()
	s.HideBubble()
	s.publish()
	go s.transcribeFile(ctx, absolute, config)
	return nil
}
func (s *Studio) CancelFile() {
	s.mu.Lock()
	if s.status == "transcribing" {
		s.status = "stopping"
		if s.cancel != nil {
			s.cancel()
		}
	}
	s.mu.Unlock()
	s.publish()
}
func (s *Studio) transcribeFile(ctx context.Context, path string, config Settings) {
	var result string
	var turns []SpeakerTurn
	var jobErr error
	defer func() {
		s.mu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.cancel = nil
		cancelled := ctx.Err() != nil && (errors.Is(jobErr, context.Canceled) || s.status == "stopping" || s.closing)
		s.fileStage = "Complete"
		s.status = "idle"
		if cancelled {
			s.fileStage = "Cancelled"
			result = ""
		} else if jobErr != nil {
			s.status = "error"
			s.problem = jobErr.Error()
			s.fileStage = "Failed"
		}
		s.transcript = result
		if cancelled {
			turns = nil
		}
		s.turns = turns
		if result != "" && jobErr == nil && !s.closing && s.saved.Settings.SaveHistory {
			entry := Entry{Turns: turns, ID: fmt.Sprint(time.Now().UnixNano()), Text: result, Model: config.Model, Language: config.Language, Created: time.Now().Format(time.RFC3339), Seconds: int(time.Since(s.started).Seconds())}
			s.saved.History = append([]Entry{entry}, s.saved.History...)
			if len(s.saved.History) > 200 {
				s.saved.History = s.saved.History[:200]
			}
			if err := s.saveLocked(); err != nil {
				s.problem = "Could not save transcript: " + err.Error()
			}
		}
		done := s.done
		s.mu.Unlock()
		s.publish()
		close(done)
	}()
	// Private scratch directory; source audio is never modified.
	temp, err := os.MkdirTemp(s.root, "import-")
	if err != nil {
		jobErr = err
		return
	}
	defer os.RemoveAll(temp)
	wav := filepath.Join(temp, "audio.wav")
	logFile, err := os.OpenFile(filepath.Join(temp, "engine.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		jobErr = err
		return
	}
	defer logFile.Close()
	convert := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-i", path, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav)
	convert.Stderr = logFile
	if err = convert.Run(); err != nil {
		jobErr = fmt.Errorf("audio conversion failed (unsupported or damaged file, or insufficient temporary disk space): %w", err)
		return
	}
	s.mu.Lock()
	s.fileStage = "Preparing model"
	s.mu.Unlock()
	s.publish()
	bin, model, err := engine.Prepare(ctx, s.root, config.Model)
	if err != nil {
		jobErr = err
		return
	}
	if ctx.Err() != nil {
		jobErr = ctx.Err()
		return
	}
	s.mu.Lock()
	s.fileStage = "Transcribing"
	s.mu.Unlock()
	s.publish()
	output := filepath.Join(temp, "transcript")
	name, args := engine.FileArgs(bin, model, config.Language, wav, output)
	if config.Diarization {
		filtered := args[:0]
		for _, arg := range args {
			if arg != "-nt" {
				filtered = append(filtered, arg)
			}
		}
		args = append(filtered, "-oj", "-ml", "1", "-sow")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = logFile
	if err = cmd.Run(); err != nil {
		jobErr = fmt.Errorf("file transcription failed: %w", err)
		return
	}
	contents, err := os.ReadFile(output + ".txt")
	if err != nil {
		jobErr = err
		return
	}
	result = strings.TrimSpace(string(contents))
	if config.Diarization {
		s.mu.Lock()
		s.fileStage = "Identifying speakers"
		s.mu.Unlock()
		s.publish()
		diarBin, diarModel, err := engine.PrepareDiarization(ctx, s.root)
		if err != nil {
			jobErr = err
			return
		}
		diarOutput := filepath.Join(temp, "speakers.json")
		diar := exec.CommandContext(ctx, diarBin, "diarize", wav, "--model", diarModel, "--preset", "v3-offline", "--backend", "cuda", "--format", "json", "--output", diarOutput)
		diar.Stderr = logFile
		if err = diar.Run(); err != nil {
			jobErr = fmt.Errorf("speaker identification failed; plain transcript retained: %w", err)
			return
		}
		turns, err = readSpeakerTurns(output+".json", diarOutput)
		if err != nil {
			jobErr = err
			return
		}
		if len(turns) > 0 {
			result = speakerText(turns)
		}
	}
}
