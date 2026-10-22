package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func detailFixture(t *testing.T) (*Studio, string) {
	t.Helper()
	s, err := newStudio(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "saved-meeting"
	folder := filepath.Join(s.root, "meetings", id)
	if err = os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err = saveMeetingRecording(&meetingSession{folder: folder, started: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"microphone.wav", "meeting-audio.wav"} {
		if err = writeWAV(filepath.Join(folder, name), make([]byte, 32000)); err != nil {
			t.Fatal(err)
		}
	}
	turns := []SpeakerTurn{{Start: 0, End: 1, Speaker: "Speaker 1", Text: "A saved conversation."}, {Start: 1, End: 2, Speaker: "You", Text: "My reply."}}
	data, _ := json.Marshal(MeetingDetail{Turns: turns, Transcript: speakerText(turns)})
	if err = os.WriteFile(filepath.Join(folder, "transcript.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return s, id
}
func TestMeetingDetailNamesSurviveRestartAndReprocessing(t *testing.T) {
	s, id := detailFixture(t)
	if err := s.RenameMeetingSpeaker(id, "Speaker 1", "Alex"); err != nil {
		t.Fatal(err)
	}
	restored, err := newStudio(s.root)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := restored.GetMeeting(id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.SpeakerNames["Speaker 1"] != "Alex" || detail.Turns[0].Speaker != "Speaker 1" {
		t.Fatal("lost name or changed stable identity", detail)
	}
	// Finish a new inference pass through the real checkpoint/commit path.
	folder := filepath.Join(s.root, "meetings", id)
	word := wordSegment{Text: "A reprocessed conversation."}
	word.Offsets.To = 500
	m := &meetingSession{folder: folder, started: time.Now(), remoteWords: []wordSegment{word}, speakers: []speakerSegment{{Start: 0, End: 1, Speaker: 1}}}
	restored.done = make(chan struct{})
	if err = restored.checkpointMeeting(m); err != nil {
		t.Fatal(err)
	}
	restored.finishMeeting(m, nil, true)
	detail, err = restored.GetMeeting(id)
	if err != nil || detail.SpeakerNames["Speaker 1"] != "Alex" {
		t.Fatal("lost names", err)
	}
	if err = restored.RenameMeetingSpeaker(id, "Speaker 1", ""); err != nil {
		t.Fatal(err)
	}
	detail, _ = restored.GetMeeting(id)
	if len(detail.SpeakerNames) != 0 {
		t.Fatal("reset did not remove custom name")
	}
}
func TestMeetingsStayOutOfStudioAndDictationHistory(t *testing.T) {
	s, id := detailFixture(t)
	folder := filepath.Join(s.root, "meetings", id)
	s.saved.History = []Entry{{ID: "legacy", MeetingFolder: folder, Text: "meeting"}, {ID: "dictation", Text: "dictation"}}
	s.transcript = "previous dictation"
	s.done = make(chan struct{})
	s.status = "meeting-processing"
	m := &meetingSession{folder: folder, started: time.Now(), config: s.saved.Settings}
	if err := s.checkpointMeeting(m); err != nil {
		t.Fatal(err)
	}
	if s.GetState().Transcript != "previous dictation" {
		t.Fatal("meeting polluted studio during processing")
	}
	s.finishMeeting(m, nil, true)
	state := s.GetState()
	if state.Transcript != "previous dictation" || len(state.Turns) != 0 || len(state.History) != 1 || state.History[0].ID != "dictation" {
		t.Fatal("meeting leaked into global state", state.History)
	}
	if len(state.Meetings) != 1 || !state.Meetings[0].Processed {
		t.Fatal("meeting missing from library")
	}
	if _, err := s.GetMeeting(id); err != nil {
		t.Fatal("meeting unavailable", err)
	}
}
func TestMeetingDetailValidation(t *testing.T) {
	s, id := detailFixture(t)
	for _, bad := range []string{"", ".", "..", "../saved-meeting", "/tmp"} {
		if _, err := s.GetMeeting(bad); err == nil {
			t.Fatal("accepted path", bad)
		}
	}
	if err := s.RenameMeetingSpeaker(id, "unknown", "Alex"); err == nil {
		t.Fatal("accepted unknown speaker")
	}
	if err := s.RenameMeetingSpeaker(id, "You", "two\nlines"); err == nil {
		t.Fatal("accepted multiline name")
	}
	s.status = "meeting-processing"
	s.meetingFolder = filepath.Join(s.root, "meetings", id)
	if err := s.RenameMeetingSpeaker(id, "You", "Alex"); err == nil {
		t.Fatal("changed names during processing")
	}
}
