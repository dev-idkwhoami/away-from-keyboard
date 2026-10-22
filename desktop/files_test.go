package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileJobGuards(t *testing.T) {
	s, _ := newStudio(t.TempDir())
	if s.TranscribeFile(t.TempDir()) == nil {
		t.Fatal("accepted directory")
	}
	for _, status := range []string{"recording", "loading", "stopping", "transcribing"} {
		s.status = status
		if s.Toggle() == nil && status == "transcribing" {
			t.Fatal("started microphone during import")
		}
		v := s.saved.Settings
		v.Model = "turbo-q5"
		if s.SetSettings(v) == nil {
			t.Fatal("changed busy model")
		}
		if s.RemoveModel("small-q5") == nil {
			t.Fatal("removed busy model")
		}
	}
}
func TestFileTranscriptionIntegration(t *testing.T) {
	data := os.Getenv("AFK_FILE_TEST_DATA")
	if data == "" {
		t.Skip("requires prepared CUDA runtime and model")
	}
	s, _ := newStudio(t.TempDir())
	for _, dir := range []string{"models", "runtime"} {
		if err := os.Symlink(filepath.Join(data, dir), filepath.Join(s.root, dir)); err != nil {
			t.Fatal(err)
		}
	}
	s.saved.Settings.Language = "de"
	start := time.Now()
	if err := s.TranscribeFile(os.Getenv("AFK_FILE_TEST_WAV")); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		s.CancelFile()
		t.Fatal("file job timed out")
	}
	state := s.GetState()
	if state.Status != "idle" || !strings.Contains(strings.ToLower(state.Transcript), "alles hat ein ende") || len(state.History) != 1 || state.BubbleVisible {
		t.Fatalf("unexpected result: %+v", state)
	}
	loaded, err := newStudio(s.root)
	if err != nil || len(loaded.GetState().History) != 1 {
		t.Fatal("history not persisted", err)
	}
	t.Logf("file import in %s: %s", time.Since(start), state.Transcript)
	bad := filepath.Join(s.root, "broken.mp3")
	os.WriteFile(bad, []byte("not audio"), 0600)
	if err = s.TranscribeFile(bad); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done = s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("bad input did not finish")
	}
	if state = s.GetState(); state.Status != "error" || len(state.History) != 1 {
		t.Fatal("bad file changed history")
	}
	leftovers, _ := filepath.Glob(filepath.Join(s.root, "import-*"))
	if len(leftovers) != 0 {
		t.Fatal("temporary audio was not cleaned up")
	}

	if err = s.TranscribeFile(os.Getenv("AFK_FILE_TEST_WAV")); err != nil {
		t.Fatal(err)
	}
	s.CancelFile()
	s.mu.Lock()
	done = s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancel did not finish")
	}
	if state = s.GetState(); state.FileStage != "Cancelled" || len(state.History) != 1 {
		t.Fatal("cancel saved partial result")
	}
}

func TestNemotronFileIntegration(t *testing.T) {
	data := os.Getenv("AFK_NEMOTRON_TEST_DATA")
	if data == "" {
		t.Skip("requires prepared Nemotron and CUDA")
	}
	s, _ := newStudio(t.TempDir())
	for _, dir := range []string{"models", "runtime", "nemotron"} {
		if err := os.Symlink(filepath.Join(data, dir), filepath.Join(s.root, dir)); err != nil {
			t.Fatal(err)
		}
	}
	s.saved.Settings.Diarization = true
	s.saved.Settings.Language = "en"
	if err := s.TranscribeFile(os.Getenv("AFK_NEMOTRON_TEST_WAV")); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Minute):
		s.CancelFile()
		t.Fatal("timed out")
	}
	state := s.GetState()
	if state.Status != "idle" || len(state.Turns) == 0 || len(state.History) != 1 || state.BubbleVisible {
		t.Fatalf("job failed: %s; turns=%d history=%d", state.Error, len(state.Turns), len(state.History))
	}
	speakers := map[string]bool{}
	for _, turn := range state.Turns {
		if strings.HasPrefix(turn.Speaker, "Speaker ") {
			speakers[turn.Speaker] = true
		}
	}
	if len(speakers) < 2 {
		t.Fatalf("expected multiple speakers, got %v: %s", speakers, state.Transcript)
	}
	loaded, err := newStudio(s.root)
	if err != nil || len(loaded.GetState().History[0].Turns) == 0 {
		t.Fatal("speaker turns not persisted", err)
	}
	t.Logf("%d turns, %d speakers: %s", len(state.Turns), len(speakers), state.Transcript)
}
