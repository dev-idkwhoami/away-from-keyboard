package main

import (
	"afk/internal/engine"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Settings struct {
	Mode           string  `json:"mode"`
	MeetingMic     string  `json:"meetingMic"`
	MeetingAudio   string  `json:"meetingAudio"`
	Diarization    bool    `json:"diarization"`
	Hotkey         string  `json:"hotkey"`
	Model          string  `json:"model"`
	Language       string  `json:"language"`
	Theme          string  `json:"theme"`
	BubbleMode     string  `json:"bubbleMode"`
	BubblePosition string  `json:"bubblePosition"`
	SaveHistory    bool    `json:"saveHistory"`
	Silence        float64 `json:"silence"`
}
type Entry struct {
	MeetingFolder string        `json:"meetingFolder,omitempty"`
	Turns         []SpeakerTurn `json:"turns,omitempty"`
	ID            string        `json:"id"`
	Text          string        `json:"text"`
	Model         string        `json:"model"`
	Language      string        `json:"language"`
	Created       string        `json:"created"`
	Seconds       int           `json:"seconds"`
}
type savedState struct {
	Settings Settings `json:"settings"`
	History  []Entry  `json:"history"`
}
type State struct {
	Meetings         []MeetingRecording `json:"meetings"`
	MeetingFolder    string             `json:"meetingFolder"`
	DiarizationReady bool               `json:"diarizationReady"`
	Turns            []SpeakerTurn      `json:"turns"`
	FileName         string             `json:"fileName"`
	FileStage        string             `json:"fileStage"`
	HotkeyActive     bool               `json:"hotkeyActive"`
	HotkeyError      string             `json:"hotkeyError"`
	Settings         Settings           `json:"settings"`
	History          []Entry            `json:"history"`
	Models           []engine.ModelInfo `json:"models"`
	Status           string             `json:"status"`
	Transcript       string             `json:"transcript"`
	Error            string             `json:"error"`
	Preparing        string             `json:"preparing"`
	Started          int64              `json:"started"`
	BubbleVisible    bool               `json:"bubbleVisible"`
}

type Studio struct {
	meeting                                *meetingSession
	meetingFolder                          string
	turns                                  []SpeakerTurn
	fileName, fileStage                    string
	trayModels                             map[string]*application.MenuItem
	trayFile                               *application.MenuItem
	hotkeyMu                               sync.Mutex
	bubbleMu                               sync.Mutex
	shortcut                               shortcutBinding
	hotkeyActive                           bool
	hotkeyError                            string
	mu                                     sync.Mutex
	root                                   string
	saved                                  savedState
	status, transcript, problem, preparing string
	started                                time.Time
	app                                    *application.App
	main, bubble                           *application.WebviewWindow
	visible                                bool
	cmd                                    *exec.Cmd
	cancel                                 context.CancelFunc
	done                                   chan struct{}
	downloadCancel                         context.CancelFunc
	closing                                bool
}

func newStudio(root string) (*Studio, error) {
	s := &Studio{root: root, status: "idle", saved: savedState{Settings: Settings{MeetingMic: defaultMicrophone, MeetingAudio: defaultMeetingAudio, Mode: "dictation", Hotkey: "Ctrl+Shift+Space", Model: "small-q5", Language: "auto", Theme: "system", BubbleMode: "wave", BubblePosition: "top-center", SaveHistory: true, Silence: 0.003}, History: []Entry{}}}
	data, err := os.ReadFile(filepath.Join(root, "desktop-state.json"))
	if err == nil {
		if err = json.Unmarshal(data, &s.saved); err != nil {
			return nil, fmt.Errorf("read desktop settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.saved.Settings.MeetingMic == "" {
		s.saved.Settings.MeetingMic = defaultMicrophone
	}
	if s.saved.Settings.MeetingAudio == "" {
		s.saved.Settings.MeetingAudio = defaultMeetingAudio
	}
	if s.saved.History == nil {
		s.saved.History = []Entry{}
	}
	if err := validateSettings(s.saved.Settings); err != nil {
		return nil, err
	}
	return s, nil
}
func validateSettings(v Settings) error {
	if v.Mode != "" && v.Mode != "dictation" && v.Mode != "meeting" {
		return errors.New("unsupported capture mode")
	}
	if _, _, err := parseHotkey(v.Hotkey); err != nil {
		return err
	}
	known := false
	for _, m := range engine.Catalogue("") {
		if m.ID == v.Model {
			known = true
		}
	}
	if !known {
		return errors.New("unknown model")
	}
	if v.Language != "auto" && v.Language != "en" && v.Language != "de" {
		return errors.New("unsupported language")
	}
	if v.Theme != "system" && v.Theme != "dark" && v.Theme != "light" {
		return errors.New("unsupported theme")
	}
	if v.BubbleMode != "wave" && v.BubbleMode != "text" {
		return errors.New("unsupported bubble mode")
	}
	if _, ok := bubblePositions[v.BubblePosition]; !ok {
		return errors.New("unsupported bubble position")
	}
	if !(v.Silence >= 0 && v.Silence <= 0.1) {
		return errors.New("invalid silence threshold")
	}
	return nil
}
func (s *Studio) saveLocked() error {
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.saved, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".state-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(s.root, "desktop-state.json"))
}
func (s *Studio) GetState() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := append([]Entry{}, s.saved.History...)
	for i := range history {
		history[i].Turns = append([]SpeakerTurn(nil), history[i].Turns...)
	}
	started := int64(0)
	if !s.started.IsZero() {
		started = s.started.UnixMilli()
	}
	return State{Meetings: s.meetingRecordingsLocked(), MeetingFolder: s.meetingFolder, DiarizationReady: engine.DiarizationReady(s.root), Turns: append([]SpeakerTurn{}, s.turns...), FileName: s.fileName, FileStage: s.fileStage, HotkeyActive: s.hotkeyActive, HotkeyError: s.hotkeyError, Settings: s.saved.Settings, History: history, Models: engine.Catalogue(s.root), Status: s.status, Transcript: s.transcript, Error: s.problem, Preparing: s.preparing, Started: started, BubbleVisible: s.visible}
}
func (s *Studio) publish() {
	if s.app != nil {
		state := s.GetState()
		s.app.Event.Emit("afk:state", state)
		s.updateTray(state)
	}
}
func (s *Studio) SetSettings(v Settings) error {
	if err := validateSettings(v); err != nil {
		return err
	}
	s.mu.Lock()
	if busyStatus(s.status) {
		if v.Mode != s.saved.Settings.Mode || v.MeetingMic != s.saved.Settings.MeetingMic || v.MeetingAudio != s.saved.Settings.MeetingAudio || v.Model != s.saved.Settings.Model || v.Language != s.saved.Settings.Language || v.Silence != s.saved.Settings.Silence || v.Diarization != s.saved.Settings.Diarization {
			s.mu.Unlock()
			return errors.New("stop recording before changing recognition settings")
		}
	}
	if v.Hotkey != s.saved.Settings.Hotkey {
		s.mu.Unlock()
		return errors.New("use the shortcut controls to change the global hotkey")
	}
	old := s.saved.Settings
	s.saved.Settings = v
	err := s.saveLocked()
	if err != nil {
		s.saved.Settings = old
	}
	s.mu.Unlock()
	if err == nil {
		s.resizeBubble()
		s.publish()
	}
	return err
}
func (s *Studio) PrepareModel(id string) error {
	found := false
	for _, m := range engine.Catalogue(s.root) {
		if m.ID == id {
			found = true
		}
	}
	if !found {
		return errors.New("unknown model")
	}
	s.mu.Lock()
	if s.preparing != "" || busyStatus(s.status) {
		s.mu.Unlock()
		return errors.New("finish the current operation first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.downloadCancel = cancel
	s.preparing = id
	s.problem = ""
	s.mu.Unlock()
	s.publish()
	go func() {
		_, _, err := engine.Prepare(ctx, s.root, id)
		cancel()
		s.mu.Lock()
		s.preparing = ""
		s.downloadCancel = nil
		if err != nil {
			s.problem = err.Error()
		}
		s.mu.Unlock()
		s.publish()
	}()
	return nil
}
func (s *Studio) RemoveModel(id string) error {
	s.mu.Lock()
	if s.preparing != "" || busyStatus(s.status) {
		s.mu.Unlock()
		return errors.New("finish the current operation first")
	}
	var err error
	if id == engine.DiarizationID {
		if s.saved.Settings.Diarization {
			s.mu.Unlock()
			return errors.New("disable Identify speakers before removing Nemotron")
		}
		err = engine.RemoveDiarization(s.root)
	} else {
		err = engine.Remove(s.root, id)
	}
	s.mu.Unlock()
	s.publish()
	return err
}
func (s *Studio) Toggle() error {
	s.mu.Lock()
	meeting, mode := s.meeting != nil, s.saved.Settings.Mode
	s.mu.Unlock()
	if meeting {
		s.StopMeeting()
		return nil
	}
	if mode == "meeting" {
		return s.StartMeeting()
	}
	return s.ToggleDictation()
}
func (s *Studio) ToggleDictation() error {
	s.mu.Lock()
	if s.status == "recording" || s.status == "loading" {
		s.status = "stopping"
		cmd, cancel, done := s.cmd, s.cancel, s.done
		s.mu.Unlock()
		s.HideBubble()
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
			go func() {
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					_ = cmd.Process.Kill()
				}
			}()
		} else if cancel != nil {
			cancel()
		}
		return nil
	}
	if busyStatus(s.status) || s.preparing != "" {
		s.mu.Unlock()
		return errors.New("finish the current operation first")
	}
	installed := false
	for _, m := range engine.Catalogue(s.root) {
		if m.ID == s.saved.Settings.Model && m.Installed {
			installed = true
		}
	}
	if !installed {
		s.mu.Unlock()
		return errors.New("download the selected model first")
	}
	config := s.saved.Settings
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	s.status = "loading"
	s.fileName, s.fileStage = "", ""
	s.meetingFolder = ""
	s.turns = nil
	s.transcript = ""
	s.problem = ""
	s.started = time.Now()
	s.mu.Unlock()
	s.publish()
	go s.record(ctx, config)
	return nil
}
func (s *Studio) record(ctx context.Context, config Settings) {
	var runErr error
	committed, pending := "", ""
	defer func() {
		s.mu.Lock()
		s.cmd = nil
		if s.cancel != nil {
			s.cancel()
		}
		s.cancel = nil
		text := strings.TrimSpace(joinText(committed, pending))
		s.transcript = text
		if runErr != nil && !errors.Is(runErr, context.Canceled) {
			s.status = "error"
			s.problem = runErr.Error()
		} else {
			s.status = "idle"
		}
		if text != "" && !s.closing {
			e := Entry{ID: fmt.Sprint(time.Now().UnixNano()), Text: text, Model: config.Model, Language: config.Language, Created: time.Now().Format(time.RFC3339), Seconds: int(time.Since(s.started).Seconds())}
			if config.SaveHistory {
				s.saved.History = append([]Entry{e}, s.saved.History...)
				if len(s.saved.History) > 200 {
					s.saved.History = s.saved.History[:200]
				}
				if err := s.saveLocked(); err != nil {
					s.problem = "Could not save history: " + err.Error()
				}
			}
		}
		closing := s.closing
		done := s.done
		s.mu.Unlock()
		s.HideBubble()
		if text != "" && !closing && s.app != nil {
			if !s.app.Clipboard.SetText(text) {
				s.mu.Lock()
				s.problem = "Clipboard copy failed. You can copy the transcript from History."
				s.mu.Unlock()
			}
		}
		s.publish()
		close(done)
	}()
	bin, path, err := engine.Prepare(ctx, s.root, config.Model)
	if err != nil {
		runErr = err
		return
	}
	if ctx.Err() != nil {
		runErr = ctx.Err()
		return
	}
	out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name", "--format=csv,noheader").CombinedOutput()
	if err != nil {
		runErr = fmt.Errorf("GPU unavailable: %s", out)
		return
	}
	name, args := engine.StreamArgs(bin, path, config.Language)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "AFK_EVENTS=1", fmt.Sprintf("AFK_SILENCE_RMS=%g", config.Silence))
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Signal(os.Interrupt)
		}
		return nil
	}
	cmd.WaitDelay = 3 * time.Second
	log, err := os.OpenFile(filepath.Join(s.root, "desktop-engine.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		runErr = err
		return
	}
	defer log.Close()
	cmd.Stderr = log
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		runErr = err
		return
	}
	s.mu.Lock()
	if s.status == "stopping" {
		s.mu.Unlock()
		return
	}
	if err = cmd.Start(); err != nil {
		s.mu.Unlock()
		runErr = err
		return
	}
	s.cmd = cmd
	s.mu.Unlock()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "[Start speaking]" {
			s.mu.Lock()
			if s.status == "loading" {
				s.status = "recording"
			}
			s.mu.Unlock()
			s.ShowBubble()
			s.publish()
			continue
		}
		var event struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		switch event.Type {
		case "partial":
			pending = event.Text
		case "commit":
			committed = joinText(committed, event.Text)
			pending = ""
		default:
			continue
		}
		s.mu.Lock()
		s.transcript = joinText(committed, pending)
		s.mu.Unlock()
		s.publish()
	}
	err = cmd.Wait()
	s.mu.Lock()
	stopping := s.status == "stopping" || s.closing
	s.mu.Unlock()
	if scanErr := scanner.Err(); scanErr != nil {
		runErr = scanErr
	} else if err != nil && !stopping {
		runErr = fmt.Errorf("transcription engine stopped: %w (see .afk/desktop-engine.log)", err)
	}
}

// Match only multi-word boundary overlaps; preserve intentional single-word repetitions.
func joinText(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	left, right := strings.Fields(a), strings.Fields(b)
	for n := min(len(left), len(right), 8); n >= 2; n-- {
		if strings.EqualFold(strings.Join(left[len(left)-n:], " "), strings.Join(right[:n], " ")) {
			return strings.TrimSpace(a + " " + strings.Join(right[n:], " "))
		}
	}
	return a + " " + b
}
func (s *Studio) CopyText(text string) error {
	if s.app == nil || !s.app.Clipboard.SetText(text) {
		return errors.New("clipboard unavailable")
	}
	return nil
}
func (s *Studio) DeleteHistory(id string) error {
	s.mu.Lock()
	old := s.saved.History
	next := []Entry{}
	for _, e := range old {
		if e.ID != id {
			next = append(next, e)
		}
	}
	s.saved.History = next
	err := s.saveLocked()
	if err != nil {
		s.saved.History = old
	}
	s.mu.Unlock()
	s.publish()
	return err
}
func (s *Studio) ClearHistory() error {
	s.mu.Lock()
	old := s.saved.History
	s.saved.History = []Entry{}
	err := s.saveLocked()
	if err != nil {
		s.saved.History = old
	}
	s.mu.Unlock()
	s.publish()
	return err
}
func (s *Studio) ShowBubble() {
	s.bubbleMu.Lock()
	defer s.bubbleMu.Unlock()
	s.mu.Lock()
	if (s.status != "recording" && s.status != "meeting") || s.closing {
		s.mu.Unlock()
		return
	}
	s.visible = true
	s.mu.Unlock()
	if s.bubble != nil {
		prepareBubbleWindow(s.bubble)
		s.resizeBubble()
		s.bubble.Show()
		s.resizeBubble()
	}
	s.publish()
}
func (s *Studio) HideBubble() {
	s.bubbleMu.Lock()
	defer s.bubbleMu.Unlock()
	s.mu.Lock()
	s.visible = false
	s.mu.Unlock()
	if s.bubble != nil {
		s.bubble.Hide()
	}
	s.publish()
}
func (s *Studio) ExpandBubble() {
	s.mu.Lock()
	if s.saved.Settings.BubbleMode == "wave" {
		s.saved.Settings.BubbleMode = "text"
	} else {
		s.saved.Settings.BubbleMode = "wave"
	}
	err := s.saveLocked()
	if err != nil {
		s.problem = err.Error()
	}
	s.mu.Unlock()
	s.resizeBubble()
	s.publish()
}
func (s *Studio) resizeBubble() {
	if s.bubble == nil {
		return
	}
	s.mu.Lock()
	text := s.saved.Settings.BubbleMode == "text"
	visible := s.visible
	position := s.saved.Settings.BubblePosition
	s.mu.Unlock()
	if !visible {
		return
	}
	w, h := 340, 92
	if text {
		w, h = 560, 210
	}
	sizeBubbleWindow(s.bubble, w, h)
	s.placeBubble(w, h, position)
}
func (s *Studio) OpenStudio() {
	if s.main != nil {
		s.main.Show()
		s.main.Focus()
	}
}
func (s *Studio) Quit() {
	if s.app != nil {
		s.app.Quit()
	}
}
func (s *Studio) ServiceShutdown() error {
	s.mu.Lock()
	s.closing = true
	cancel, download, done := s.cancel, s.downloadCancel, s.done
	s.mu.Unlock()
	s.hotkeyMu.Lock()
	if s.shortcut != nil {
		_ = s.shortcut.Close()
	}
	s.hotkeyMu.Unlock()
	if download != nil {
		download()
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(4 * time.Second):
		}
	}
	return nil
}
