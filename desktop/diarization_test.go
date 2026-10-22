package main

import (
	"strings"
	"testing"
)

func word(text string, start, end int64) wordSegment {
	w := wordSegment{Text: text}
	w.Offsets.From = start
	w.Offsets.To = end
	return w
}
func TestSpeakerAlignment(t *testing.T) {
	words := []wordSegment{word("Hello", 0, 500), word("there.", 500, 1000), word("Guten", 1100, 1500), word("Tag.", 1500, 2000), word("unclear", 2100, 2400), word("both", 2500, 2800)}
	speakers := []speakerSegment{{0, 1, 1}, {1.1, 2, 2}, {2.5, 2.8, 1}, {2.5, 2.8, 2}}
	got := alignSpeakers(words, speakers)
	if len(got) != 4 || got[0].Speaker != "Speaker 1" || got[0].Text != "Hello there." || got[1].Speaker != "Speaker 2" || got[2].Speaker != "Unassigned" || got[3].Speaker != "Overlapping speakers" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(speakerText(got), "[00:00:01] Speaker 2: Guten Tag.") {
		t.Fatal(speakerText(got))
	}
}
func TestSpeakerAlignmentNoSpeechAndInvalidTimes(t *testing.T) {
	got := alignSpeakers([]wordSegment{word("kept", 1000, 1100), word("bad", -1, 10)}, nil)
	if len(got) != 1 || got[0].Speaker != "Unassigned" || got[0].Text != "kept" {
		t.Fatal(got)
	}
	if got := alignSpeakers(nil, []speakerSegment{{0, 1, 1}}); len(got) != 0 {
		t.Fatal(got)
	}
}
func TestSpeakerTurnsPersistAndDoNotLeak(t *testing.T) {
	s, _ := newStudio(t.TempDir())
	settings := s.GetState().Settings
	settings.Diarization = true
	if err := s.SetSettings(settings); err != nil {
		t.Fatal(err)
	}
	s.saved.History = []Entry{{ID: "meeting", Text: "words", Turns: []SpeakerTurn{{0, 1, "Speaker 1", "words"}}}}
	if err := s.saveLocked(); err != nil {
		t.Fatal(err)
	}
	loaded, err := newStudio(s.root)
	if err != nil {
		t.Fatal(err)
	}
	state := loaded.GetState()
	if !state.Settings.Diarization {
		t.Fatal("speaker setting not persisted")
	}
	state.History[0].Turns[0].Text = "changed"
	if loaded.GetState().History[0].Turns[0].Text != "words" {
		t.Fatal("mutable speaker history leaked")
	}
}
