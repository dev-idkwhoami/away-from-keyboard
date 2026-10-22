package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAudioResolution(t *testing.T) {
	sources := []AudioSource{{ID: "mic-a", Device: "mic-a", Kind: "mic", Label: "Microphone A"}, {ID: "mic-b", Device: "mic-b", Kind: "mic"}, {ID: "monitor-a", Device: "monitor-a", Kind: "output"}, {ID: "monitor-b", Device: "monitor-b", Kind: "output"}}
	sinks := []audioNode{{Name: "sink-a", Monitor: "monitor-a", Description: "Output A"}, {Name: "sink-b", Monitor: "monitor-b"}}
	config := Settings{MeetingMic: defaultMicrophone, MeetingAudio: defaultMeetingAudio}
	first := withAudioDefaults(sources, sinks, "mic-a", "sink-a")
	mic, remote, err := resolveMeetingSources(first, config)
	if err != nil || mic.ID != "mic-a" || mic.Device != "mic-a" || remote.ID != "monitor-a" || remote.Device != "monitor-a" {
		t.Fatalf("wrong default mapping: %+v %+v %v", mic, remote, err)
	}
	second := withAudioDefaults(sources, sinks, "mic-b", "sink-b")
	nextMic, nextRemote, err := resolveMeetingSources(second, config)
	if err != nil || nextMic.ID != "mic-b" || nextRemote.ID != "monitor-b" {
		t.Fatal("next recording did not refresh defaults")
	}
	pinned := Settings{MeetingMic: mic.ID, MeetingAudio: remote.ID}
	sameMic, sameRemote, err := resolveMeetingSources(second, pinned)
	if err != nil || sameMic.ID != mic.ID || sameRemote.ID != remote.ID {
		t.Fatal("default change affected pinned recording")
	}
	unavailable := withAudioDefaults(sources, sinks, "missing", "missing")
	if _, _, err = resolveMeetingSources(unavailable, config); err == nil {
		t.Fatal("unavailable defaults silently substituted")
	}
}
func TestDefaultAudioSettingsMigration(t *testing.T) {
	root := t.TempDir()
	s, err := newStudio(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.saved.Settings.MeetingMic != defaultMicrophone || s.saved.Settings.MeetingAudio != defaultMeetingAudio {
		t.Fatal("missing defaults on fresh install")
	}
	s.saved.Settings.MeetingMic = ""
	s.saved.Settings.MeetingAudio = "explicit-output"
	if err = s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStudio(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.saved.Settings.MeetingMic != defaultMicrophone || loaded.saved.Settings.MeetingAudio != "explicit-output" {
		t.Fatal("migration lost explicit selection")
	}
	if _, err = os.Stat(filepath.Join(root, "desktop-state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSystemAudioDefaults(t *testing.T) {
	if os.Getenv("AFK_DEFAULT_AUDIO_TEST") == "" {
		t.Skip("requires the desktop audio server; metadata only")
	}
	s, _ := newStudio(t.TempDir())
	sources, err := s.GetAudioSources()
	if err != nil {
		t.Fatal(err)
	}
	mic, output, err := resolveMeetingSources(sources, s.saved.Settings)
	if err != nil || mic.Device == "" || output.Device == "" || mic.ID == defaultMicrophone || output.ID == defaultMeetingAudio {
		t.Fatalf("system defaults failed to resolve: %v", err)
	}
	t.Log("System default microphone and output resolved to available concrete devices; no capture started")
}
