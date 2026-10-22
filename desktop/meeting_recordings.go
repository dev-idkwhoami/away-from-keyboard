package main

import (
	"afk/internal/engine"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MeetingRecording struct {
	ID        string    `json:"id"`
	Started   time.Time `json:"started"`
	Seconds   int       `json:"seconds"`
	Processed bool      `json:"processed"`
	Offsets   [2]int64  `json:"offsets"`
}

func meetingPCMSize(f *os.File) (int64, error) {
	h := make([]byte, 44)
	if _, e := f.ReadAt(h, 0); e != nil {
		return 0, e
	}
	if string(h[:4]) != "RIFF" || string(h[8:16]) != "WAVEfmt " || string(h[36:40]) != "data" || binary.LittleEndian.Uint16(h[20:]) != 1 || binary.LittleEndian.Uint16(h[22:]) != 1 || binary.LittleEndian.Uint32(h[24:]) != 16000 || binary.LittleEndian.Uint16(h[34:]) != 16 {
		return 0, errors.New("invalid meeting PCM audio")
	}
	st, e := f.Stat()
	if e != nil {
		return 0, e
	}
	size := int64(binary.LittleEndian.Uint32(h[40:]))
	if size > st.Size()-44 || size%2 != 0 {
		return 0, io.ErrUnexpectedEOF
	}
	return size, nil
}
func saveMeetingRecording(m *meetingSession) error {
	record := MeetingRecording{ID: filepath.Base(m.folder), Started: m.started, Seconds: int(time.Since(m.started).Seconds()), Offsets: m.offsets}
	data, e := json.MarshalIndent(record, "", "  ")
	if e != nil {
		return e
	}
	path := filepath.Join(m.folder, "recording.json")
	if e = os.WriteFile(path+".tmp", data, 0600); e != nil {
		return e
	}
	return os.Rename(path+".tmp", path)
}
func readMeetingRecording(folder string) (MeetingRecording, error) {
	var record MeetingRecording
	data, err := os.ReadFile(filepath.Join(folder, "recording.json"))
	if os.IsNotExist(err) {
		data, err = os.ReadFile(filepath.Join(folder, "transcript.json"))
	} // Earlier live recordings.
	if err != nil {
		return record, err
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	record.ID = filepath.Base(folder)
	record.Seconds = 0
	for _, name := range []string{"microphone.wav", "meeting-audio.wav"} {
		f, e := os.Open(filepath.Join(folder, name))
		if e != nil {
			return record, e
		}
		size, e := meetingPCMSize(f)
		f.Close()
		if e != nil {
			return record, e
		}
		record.Seconds = max(record.Seconds, int(size/32000))
	}
	_, err = os.Stat(filepath.Join(folder, "processed"))
	record.Processed = err == nil
	return record, nil
}
func (s *Studio) meetingRecordingsLocked() []MeetingRecording {
	records := []MeetingRecording{}
	dirs, _ := os.ReadDir(filepath.Join(s.root, "meetings"))
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		if s.meeting != nil && dir.Name() == filepath.Base(s.meetingFolder) {
			continue
		}
		record, e := readMeetingRecording(filepath.Join(s.root, "meetings", dir.Name()))
		if e == nil {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID > records[j].ID })
	return records
}
func (s *Studio) ProcessMeeting(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if busyStatus(s.status) || s.preparing != "" || s.closing {
		return errors.New("finish the current operation first")
	}
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, "/\\") {
		return errors.New("invalid meeting recording")
	}
	folder := filepath.Join(s.root, "meetings", id)
	info, err := os.Lstat(folder)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("invalid meeting folder")
	}
	record, err := readMeetingRecording(folder)
	if err != nil {
		return err
	}
	if !engine.DiarizationReady(s.root) {
		return errors.New("prepare Nemotron in Model Garage before processing")
	}
	installed := false
	for _, model := range engine.Catalogue(s.root) {
		if model.ID == s.saved.Settings.Model && model.Installed {
			installed = true
		}
	}
	if !installed {
		return errors.New("download the selected Whisper model first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &meetingSession{folder: folder, started: record.Started, offsets: record.Offsets, config: s.saved.Settings}
	s.cancel = cancel
	s.done = make(chan struct{})
	s.status = "meeting-processing"
	s.problem = ""
	s.transcript = ""
	s.turns = nil
	s.meetingFolder = folder
	s.fileName = ""
	s.fileStage = ""
	go func() { s.publish(); s.processMeeting(ctx, m); cancel() }()
	return nil
}
func (s *Studio) CancelMeetingProcessing() {
	s.mu.Lock()
	if s.status == "meeting-processing" {
		s.status = "meeting-cancelling"
		if s.cancel != nil {
			s.cancel()
		}
	}
	s.mu.Unlock()
	s.publish()
}
func (s *Studio) finishMeeting(m *meetingSession, runErr error, processing bool) {
	if m.captureCancel != nil {
		m.captureCancel()
	}
	s.mu.Lock()
	cancelled := s.closing || s.status == "meeting-cancelling"
	s.status = "idle"
	if runErr != nil && !errors.Is(runErr, context.Canceled) && !cancelled {
		s.status = "error"
		s.problem = runErr.Error()
	}
	if processing && runErr == nil && !cancelled {
		if err := os.Rename(filepath.Join(m.folder, "transcript.pending.json"), filepath.Join(m.folder, "transcript.json")); err != nil {
			s.problem = err.Error()
			s.status = "error"
			runErr = err
		}

		if runErr == nil {
			if err := os.WriteFile(filepath.Join(m.folder, "processed"), []byte(time.Now().Format(time.RFC3339)), 0600); err != nil {
				s.problem = err.Error()
				s.status = "error"
			}
		}
		if runErr == nil && s.transcript != "" && m.config.SaveHistory {
			// Reprocessing replaces this meeting's history entry.
			history := []Entry{}
			for _, entry := range s.saved.History {
				if entry.MeetingFolder != m.folder {
					history = append(history, entry)
				}
			}
			record, _ := readMeetingRecording(m.folder)
			s.saved.History = append([]Entry{{MeetingFolder: m.folder, ID: fmt.Sprint(time.Now().UnixNano()), Text: s.transcript, Turns: append([]SpeakerTurn(nil), s.turns...), Model: m.config.Model, Language: m.config.Language, Created: m.started.Format(time.RFC3339), Seconds: record.Seconds}}, history...)
			if len(s.saved.History) > 200 {
				s.saved.History = s.saved.History[:200]
			}
			if err := s.saveLocked(); err != nil {
				s.problem = err.Error()
			}
		}
	}
	s.meeting = nil
	s.cancel = nil
	done := s.done
	s.mu.Unlock()
	s.HideBubble()
	s.publish()
	close(done)
}
