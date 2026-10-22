package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type MeetingDetail struct {
	Recording    MeetingRecording  `json:"recording"`
	Turns        []SpeakerTurn     `json:"turns"`
	Transcript   string            `json:"transcript"`
	SpeakerNames map[string]string `json:"speakerNames"`
}

func (s *Studio) meetingPath(id string) (string, error) {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, "/\\") {
		return "", errors.New("invalid meeting recording")
	}
	folder := filepath.Join(s.root, "meetings", id)
	info, err := os.Lstat(folder)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("invalid meeting folder")
	}
	return folder, nil
}
func (s *Studio) GetMeeting(id string) (MeetingDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meetingDetailLocked(id)
}
func (s *Studio) meetingDetailLocked(id string) (MeetingDetail, error) {
	detail := MeetingDetail{Turns: []SpeakerTurn{}, SpeakerNames: map[string]string{}}
	folder, err := s.meetingPath(id)
	if err != nil {
		return detail, err
	}
	detail.Recording, err = readMeetingRecording(folder)
	if err != nil {
		return detail, err
	}
	data, err := os.ReadFile(filepath.Join(folder, "transcript.json"))
	if err == nil {
		if err = json.Unmarshal(data, &detail); err != nil {
			return detail, err
		}
	} else if !os.IsNotExist(err) {
		return detail, err
	}
	data, err = os.ReadFile(filepath.Join(folder, "speakers.json"))
	if err == nil {
		if err = json.Unmarshal(data, &detail.SpeakerNames); err != nil {
			return detail, err
		}
	} else if !os.IsNotExist(err) {
		return detail, err
	}
	if detail.Turns == nil {
		detail.Turns = []SpeakerTurn{}
	}
	if detail.SpeakerNames == nil {
		detail.SpeakerNames = map[string]string{}
	}
	return detail, nil
}
func (s *Studio) RenameMeetingSpeaker(id, speaker, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.meetingFolder == filepath.Join(s.root, "meetings", id) && busyStatus(s.status) {
		return errors.New("wait for this meeting to finish processing")
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 80 || strings.ContainsAny(name, "\r\n\x00") {
		return errors.New("speaker names must be a single line of at most 80 characters")
	}
	detail, err := s.meetingDetailLocked(id)
	if err != nil {
		return err
	}
	found := false
	for _, turn := range detail.Turns {
		if turn.Speaker == speaker {
			found = true
			break
		}
	}
	if !found {
		return errors.New("unknown speaker")
	}
	if name == "" || name == speaker {
		delete(detail.SpeakerNames, speaker)
	} else {
		detail.SpeakerNames[speaker] = name
	}
	data, err := json.MarshalIndent(detail.SpeakerNames, "", "  ")
	if err != nil {
		return err
	}
	folder, _ := s.meetingPath(id)
	f, err := os.CreateTemp(folder, ".speakers-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(folder, "speakers.json"))
}
