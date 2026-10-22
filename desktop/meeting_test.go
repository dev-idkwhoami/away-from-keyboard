package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMeetingWordBoundary(t *testing.T) {
	words := []wordSegment{{Text: "old"}, {Text: "new"}, {Text: "next"}}
	words[0].Offsets.From = 0
	words[0].Offsets.To = 600
	words[1].Offsets.From = 700
	words[1].Offsets.To = 1400
	words[2].Offsets.From = 8400
	words[2].Offsets.To = 9000
	got := meetingWords(words, 7000, 8000, 16000)
	if len(got) != 2 || got[0].Text != "new" || got[0].Offsets.From != 7700 {
		t.Fatalf("overlap duplicated or new word dropped: %+v", got)
	}
}
func TestMeetingWAVLimits(t *testing.T) {
	h, err := wavHeader(32000)
	if err != nil || binary.LittleEndian.Uint32(h[40:]) != 32000 || binary.LittleEndian.Uint32(h[4:]) != 32036 {
		t.Fatal("invalid audio checkpoint header", err)
	}
	for _, size := range []int64{-1, 1, 1 << 32} {
		if _, err = wavHeader(size); err == nil {
			t.Fatal("accepted invalid WAV size", size)
		}
	}
}
func TestMeetingSourceAndBusyGuards(t *testing.T) {
	sources := []AudioSource{{ID: "mic", Kind: "mic"}, {ID: "app:1:100", Kind: "app"}, {ID: "output", Kind: "output"}}
	config := Settings{MeetingMic: "mic", MeetingAudio: "app:1:100"}
	if _, _, err := resolveMeetingSources(sources, config); err != nil {
		t.Fatal(err)
	}
	config.MeetingAudio = "app:1:99"
	if _, _, err := resolveMeetingSources(sources, config); err == nil {
		t.Fatal("accepted recycled app stream")
	}
	config.MeetingAudio = "mic"
	if _, _, err := resolveMeetingSources(sources, config); err == nil {
		t.Fatal("accepted mic as remote")
	}
	s, _ := newStudio(t.TempDir())
	for _, status := range []string{"meeting-loading", "meeting", "meeting-stopping"} {
		s.status = status
		if s.ToggleDictation() == nil || s.StartMeeting() == nil || s.RemoveModel("small-q5") == nil {
			t.Fatal("meeting not exclusive", status)
		}
		v := s.saved.Settings
		v.Mode = "meeting"
		if s.SetSettings(v) == nil {
			t.Fatal("changed mode while active")
		}
	}
}
func TestLiveMeetingIntegration(t *testing.T) {
	data := os.Getenv("AFK_MEETING_TEST_DATA")
	if data == "" {
		t.Skip("requires virtual PulseAudio sources and prepared CUDA models")
	}
	s, err := newStudio(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.saved.Settings.Language = "en"
	s.saved.Settings.Mode = "meeting"
	s.saved.Settings.MeetingMic = os.Getenv("AFK_MEETING_TEST_MIC")
	s.saved.Settings.MeetingAudio = os.Getenv("AFK_MEETING_TEST_REMOTE")
	if err = s.Toggle(); err != nil {
		t.Fatal(err)
	}
	defer s.ServiceShutdown()
	deadline := time.Now().Add(30 * time.Second)
	for s.GetState().Status != "meeting" {
		if st := s.GetState(); st.Status == "error" {
			t.Fatal(st.Error)
		}
		if time.Now().After(deadline) {
			t.Fatal("startup timed out")
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	// Fixtures are played only into isolated virtual outputs, never speakers/microphones.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	remote := exec.CommandContext(ctx, "paplay", "--device="+os.Getenv("AFK_MEETING_TEST_REMOTE_SINK"), os.Getenv("AFK_MEETING_TEST_REMOTE_WAV"))
	mic := exec.CommandContext(ctx, "paplay", "--device="+os.Getenv("AFK_MEETING_TEST_MIC_SINK"), os.Getenv("AFK_MEETING_TEST_MIC_WAV"))
	if err = remote.Start(); err != nil {
		t.Fatal(err)
	}
	if err = mic.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { remote.Process.Kill(); mic.Process.Kill() }()
	if err = mic.Wait(); err != nil {
		t.Fatal(err)
	}
	if err = remote.Wait(); err != nil {
		t.Fatal(err)
	}
	if st := s.GetState(); st.Transcript != "" || len(st.Turns) != 0 || len(st.History) != 0 {
		t.Fatal("meeting ran inference while recording")
	}
	if err = s.Toggle(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("stop did not drain final audio")
	}
	st := s.GetState()
	if st.Status != "idle" || len(st.History) != 0 || len(st.Meetings) != 1 || st.Meetings[0].Processed {
		t.Fatal("recording was not saved for later", st.Error)
	}
	for _, dir := range []string{"models", "runtime", "nemotron"} {
		if err = os.Symlink(filepath.Join(data, dir), filepath.Join(s.root, dir)); err != nil {
			t.Fatal(err)
		}
	}
	// Restart the service before requesting processing, and preserve raw audio on cancel.
	s, err = newStudio(s.root)
	defer s.ServiceShutdown()
	if err != nil {
		t.Fatal(err)
	}
	id := s.GetState().Meetings[0].ID
	if err = s.ProcessMeeting(id); err != nil {
		t.Fatal(err)
	}
	s.CancelMeetingProcessing()
	s.mu.Lock()
	done = s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("processing cancellation timed out")
	}
	if st = s.GetState(); len(st.History) != 0 || st.Meetings[0].Processed {
		t.Fatal("cancelled processing saved a completed result")
	}
	if err = s.ProcessMeeting(id); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	done = s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(60 * time.Second):
		t.Fatal("on-demand processing timed out")
	}
	st = s.GetState()
	if st.Status != "idle" || len(st.History) != 1 || st.BubbleVisible {
		t.Fatalf("bad completion: %s %s", st.Status, st.Error)
	}
	speakers := map[string]bool{}
	for _, turn := range st.Turns {
		speakers[turn.Speaker] = true
	}
	if !speakers["You"] || len(speakers) < 3 {
		t.Fatalf("missing mic/remote speakers: %v", speakers)
	}
	for _, name := range []string{"microphone.wav", "meeting-audio.wav"} {
		b, err := os.ReadFile(filepath.Join(st.MeetingFolder, name))
		if err != nil {
			t.Fatal(err)
		}
		if len(b) < 44+32000 || int(binary.LittleEndian.Uint32(b[40:])) != len(b)-44 {
			t.Fatal("audio checkpoint was truncated", name)
		}
	}
	b, err := os.ReadFile(filepath.Join(st.MeetingFolder, "transcript.json"))
	if err != nil {
		t.Fatal(err)
	}
	var checkpoint struct{ Transcript string }
	if json.Unmarshal(b, &checkpoint) != nil || checkpoint.Transcript != st.Transcript {
		t.Fatal("checkpoint differs from final transcript")
	}
	t.Logf("On-demand meeting saved %d turns; speakers %v; transcript: %s", len(st.Turns), speakers, st.Transcript)
}

func TestMeetingLifecycleIntegration(t *testing.T) {
	data := os.Getenv("AFK_MEETING_TEST_DATA")
	if data == "" {
		t.Skip("requires virtual audio and CUDA")
	}
	s, _ := newStudio(t.TempDir())
	for _, dir := range []string{"models", "runtime", "nemotron"} {
		if err := os.Symlink(filepath.Join(data, dir), filepath.Join(s.root, dir)); err != nil {
			t.Fatal(err)
		}
	}
	s.saved.Settings.MeetingMic = os.Getenv("AFK_MEETING_TEST_MIC")
	s.saved.Settings.MeetingAudio = os.Getenv("AFK_MEETING_TEST_REMOTE")
	defer s.ServiceShutdown()
	wait := func() {
		t.Helper()
		s.mu.Lock()
		done := s.done
		s.mu.Unlock()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("meeting failed to stop")
		}
	}
	if err := s.StartMeeting(); err != nil {
		t.Fatal(err)
	}
	s.StopMeeting()
	wait()
	if st := s.GetState(); st.Status != "idle" || st.BubbleVisible {
		t.Fatalf("startup cancellation failed: %s %s", st.Status, st.Error)
	}
	id, err := exec.Command("pactl", "load-module", "module-null-sink", "sink_name=afk_disconnect_test").Output()
	if err != nil {
		t.Fatal(err)
	}
	module := strings.TrimSpace(string(id))
	defer exec.Command("pactl", "unload-module", module).Run()
	s.saved.Settings.MeetingAudio = "afk_disconnect_test.monitor"
	if err = s.StartMeeting(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for s.GetState().Status != "meeting" {
		if st := s.GetState(); st.Status == "error" {
			t.Fatal(st.Error)
		}
		if time.Now().After(deadline) {
			t.Fatal("capture not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err = exec.Command("pactl", "unload-module", module).Run(); err != nil {
		t.Fatal(err)
	}
	wait()
	if st := s.GetState(); st.Status != "error" || st.Error == "" || st.BubbleVisible {
		t.Fatalf("source removal was not reported: %+v", st)
	}
	t.Log("Startup cancellation and source disconnection handled; partial audio retained")
}

func TestSavedMeetingDiscovery(t *testing.T) {
	s, _ := newStudio(t.TempDir())
	folder := filepath.Join(s.root, "meetings", "20261022-test")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	m := &meetingSession{folder: folder, started: time.Now(), offsets: [2]int64{12, 18}}
	if err := saveMeetingRecording(m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"microphone.wav", "meeting-audio.wav"} {
		if err := writeWAV(filepath.Join(folder, name), make([]byte, 64000)); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := newStudio(s.root)
	if err != nil {
		t.Fatal(err)
	}
	records := restored.GetState().Meetings
	if len(records) != 1 || records[0].Seconds != 2 || records[0].Offsets != m.offsets || records[0].Processed {
		t.Fatalf("recording not recovered: %+v", records)
	}
	for _, id := range []string{"../outside", ".", "..", "/tmp/recording"} {
		if restored.ProcessMeeting(id) == nil {
			t.Fatal("accepted unsafe recording ID", id)
		}
	}
	if err = os.WriteFile(filepath.Join(folder, "meeting-audio.wav"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(restored.GetState().Meetings) != 0 {
		t.Fatal("offered corrupted recording")
	}
}

func TestProcessSavedMeetingIntegration(t *testing.T) {
	data := os.Getenv("AFK_MEETING_TEST_DATA")
	if data == "" {
		t.Skip("requires prepared CUDA models")
	}
	s, _ := newStudio(t.TempDir())
	defer s.ServiceShutdown()
	for _, dir := range []string{"models", "runtime", "nemotron"} {
		if e := os.Symlink(filepath.Join(data, dir), filepath.Join(s.root, dir)); e != nil {
			t.Fatal(e)
		}
	}
	folder := filepath.Join(s.root, "meetings", "processing-test")
	if e := os.MkdirAll(folder, 0700); e != nil {
		t.Fatal(e)
	}
	m := &meetingSession{folder: folder, started: time.Now()}
	if e := saveMeetingRecording(m); e != nil {
		t.Fatal(e)
	}
	for name, env := range map[string]string{"microphone.wav": "AFK_MEETING_TEST_MIC_WAV", "meeting-audio.wav": "AFK_MEETING_TEST_REMOTE_WAV"} {
		pcm, e := exec.Command("ffmpeg", "-v", "error", "-i", os.Getenv(env), "-ac", "1", "-ar", "16000", "-f", "s16le", "-").Output()
		if e != nil {
			t.Fatal(e)
		}
		if e = writeWAV(filepath.Join(folder, name), pcm); e != nil {
			t.Fatal(e)
		}
	}
	s.saved.Settings.Language = "auto"
	for attempt := 0; attempt < 2; attempt++ {
		if e := s.ProcessMeeting("processing-test"); e != nil {
			t.Fatal(e)
		}
		s.mu.Lock()
		done := s.done
		s.mu.Unlock()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("processing timed out")
		}
		st := s.GetState()
		if st.Status != "idle" || len(st.History) != 1 || !st.Meetings[0].Processed {
			t.Fatalf("bad processing result: %s %s", st.Status, st.Error)
		}
		you, remote := false, false
		for _, turn := range st.Turns {
			you = you || turn.Speaker == "You"
			remote = remote || strings.HasPrefix(turn.Speaker, "Speaker ")
		}
		if !you || !remote {
			t.Fatal("missing microphone/remote attribution")
		}
	}
	completed, e := os.ReadFile(filepath.Join(folder, "transcript.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ProcessMeeting("processing-test"); e != nil {
		t.Fatal(e)
	}
	s.CancelMeetingProcessing()
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("cancel timed out")
	}
	preserved, _ := os.ReadFile(filepath.Join(folder, "transcript.json"))
	if string(completed) != string(preserved) || len(s.GetState().History) != 1 {
		t.Fatal("cancelled retry destroyed completed transcript")
	}
	t.Log("On-demand processing, replacement history and cancelled-retry preservation passed")
}
