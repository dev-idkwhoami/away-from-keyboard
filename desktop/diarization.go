package main

import (
	"afk/internal/engine"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
)

type SpeakerTurn struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker string  `json:"speaker"`
	Text    string  `json:"text"`
}
type speakerSegment struct {
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
	Speaker int     `json:"speaker"`
}
type wordSegment struct {
	Text    string `json:"text"`
	Offsets struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"offsets"`
}

func alignSpeakers(words []wordSegment, speakers []speakerSegment) []SpeakerTurn {
	sort.SliceStable(words, func(i, j int) bool { return words[i].Offsets.From < words[j].Offsets.From })
	sort.SliceStable(speakers, func(i, j int) bool { return speakers[i].Start < speakers[j].Start })
	cursor := 0
	turns := []SpeakerTurn{}
	for _, word := range words {
		text := strings.TrimSpace(word.Text)
		if text == "" {
			continue
		}
		start, end := float64(word.Offsets.From)/1000, float64(word.Offsets.To)/1000
		if start < 0 || end < start {
			continue
		}
		overlap := map[int]float64{}
		for cursor < len(speakers) && speakers[cursor].End <= start {
			cursor++
		}
		for _, seg := range speakers[cursor:] {
			if seg.Start >= end {
				break
			}
			if seg.Speaker < 1 || seg.Speaker > 8 || math.IsNaN(seg.Start) || math.IsNaN(seg.End) {
				continue
			}
			amount := math.Max(0, math.Min(end, seg.End)-math.Max(start, seg.Start))
			if amount > 0 {
				overlap[seg.Speaker] += amount
			}
		}
		best := 0
		score := 0.0
		for id, amount := range overlap {
			if amount > score || (amount == score && id < best) {
				best, score = id, amount
			}
		}
		label := "Unassigned"
		if best != 0 {
			label = fmt.Sprintf("Speaker %d", best)
			// Avoid pretending overlapping voices can be attributed reliably.
			for id, amount := range overlap {
				if id != best && amount >= score*.8 {
					label = "Overlapping speakers"
					break
				}
			}
		}
		n := len(turns)
		if n > 0 && turns[n-1].Speaker == label && start-turns[n-1].End <= 2 {
			turns[n-1].End = math.Max(turns[n-1].End, end)
			turns[n-1].Text += " " + text
		} else {
			turns = append(turns, SpeakerTurn{start, end, label, text})
		}
	}
	return turns
}
func readSpeakerTurns(whisperPath, diarPath string) ([]SpeakerTurn, error) {
	var whisper struct {
		Transcription []wordSegment `json:"transcription"`
	}
	var diar struct {
		Segments []speakerSegment `json:"segments"`
	}
	for path, target := range map[string]interface{}{whisperPath: &whisper, diarPath: &diar} {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, target); err != nil {
			return nil, fmt.Errorf("invalid inference output: %w", err)
		}
	}
	if whisper.Transcription == nil || diar.Segments == nil {
		return nil, errors.New("inference output is missing timestamped segments")
	}
	return alignSpeakers(whisper.Transcription, diar.Segments), nil
}
func speakerText(turns []SpeakerTurn) string {
	lines := make([]string, 0, len(turns))
	for _, turn := range turns {
		seconds := int(turn.Start)
		lines = append(lines, fmt.Sprintf("[%02d:%02d:%02d] %s: %s", seconds/3600, seconds/60%60, seconds%60, turn.Speaker, turn.Text))
	}
	return strings.Join(lines, "\n\n")
}
func (s *Studio) PrepareDiarization() error {
	s.mu.Lock()
	if s.preparing != "" || (s.status != "idle" && s.status != "error") || s.closing {
		s.mu.Unlock()
		return errors.New("finish the current operation first")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.downloadCancel = cancel
	s.preparing = engine.DiarizationID
	s.problem = ""
	s.mu.Unlock()
	s.publish()
	go func() {
		_, _, err := engine.PrepareDiarization(ctx, s.root)
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
func (s *Studio) CancelPreparation() {
	s.mu.Lock()
	if s.downloadCancel != nil {
		s.downloadCancel()
	}
	s.mu.Unlock()
}
