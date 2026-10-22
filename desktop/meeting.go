package main

import (
	"afk/internal/engine"
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

func busyStatus(status string) bool { return status != "idle" && status != "error" }

type meetingSession struct {
	offsets               [2]int64
	captureCancel         context.CancelFunc
	mu                    sync.Mutex
	micWords, remoteWords []wordSegment
	speakers              []speakerSegment
	remoteOffset          int64
	folder                string
	config                Settings
	started               time.Time
}
type meetingTrack struct {
	level    func([]byte)
	ready    chan struct{}
	file     *os.File
	source   AudioSource
	mu       sync.Mutex
	size     int64
	finished bool
	offset   int64
}

func (s *Studio) StartMeeting() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if busyStatus(s.status) || s.preparing != "" || s.closing {
		return errors.New("finish the current operation first")
	}
	config := s.saved.Settings
	if config.MeetingMic == "" || config.MeetingAudio == "" {
		return errors.New("select a microphone and meeting audio source")
	}
	ctx, cancel := context.WithCancel(context.Background())
	captureCtx, captureCancel := context.WithCancel(ctx)
	m := &meetingSession{captureCancel: captureCancel, config: config, started: time.Now()}
	s.meeting = m
	s.cancel = cancel
	s.done = make(chan struct{})
	s.status = "meeting-loading"
	s.problem = ""
	s.fileName = ""
	s.fileStage = ""
	s.meetingFolder = ""
	s.started = m.started
	go func() { s.publish(); s.runMeeting(captureCtx, m); cancel() }()
	return nil
}
func (s *Studio) StopMeeting() {
	s.mu.Lock()
	if s.meeting != nil {
		s.status = "meeting-stopping"
		s.meeting.captureCancel()
	}
	s.mu.Unlock()
	s.HideBubble()
	s.publish()
}
func resolveMeetingSources(sources []AudioSource, config Settings) (AudioSource, AudioSource, error) {
	var mic, remote AudioSource
	for _, source := range sources {
		if source.ID == config.MeetingMic && source.Kind == "mic" {
			mic = source
		}
		if source.ID == config.MeetingAudio && (source.Kind == "output" || source.Kind == "app") {
			remote = source
		}
	}
	if mic.ID == defaultMicrophone {
		mic.ID = mic.Device
	}
	if remote.ID == defaultMeetingAudio {
		remote.ID = remote.Device
	}
	if mic.ID == "" || remote.ID == "" {
		return mic, remote, errors.New("a selected audio source is no longer available; refresh the source list")
	}
	return mic, remote, nil
}

// A standard mono PCM WAV, checkpointed while recording. Refuse to exceed RIFF's limit.
func wavHeader(size int64) ([]byte, error) {
	if size < 0 || size > math.MaxUint32-36 || size%2 != 0 {
		return nil, errors.New("recording reached the WAV size limit")
	}
	b := make([]byte, 44)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(size+36))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 16000)
	binary.LittleEndian.PutUint32(b[28:], 32000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(size))
	return b, nil
}
func writeWAV(path string, pcm []byte) error {
	h, err := wavHeader(int64(len(pcm)))
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(h, pcm...), 0600)
}
func (t *meetingTrack) capture(ctx context.Context, started time.Time) error {
	defer func() { t.mu.Lock(); t.finished = true; t.mu.Unlock() }()
	args := []string{"--raw", "--format=s16le", "--rate=16000", "--channels=1", "--latency-msec=50", "--client-name=AFK Meeting", "--device=" + t.source.Device}
	if t.source.Kind == "app" {
		args = append(args, "--monitor-stream="+strconv.FormatUint(uint64(t.source.Stream), 10))
	}
	cmd := exec.CommandContext(ctx, "parec", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	t.offset = time.Since(started).Milliseconds()
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	b := make([]byte, 1600)
	for {
		n, readErr := io.ReadFull(stdout, b)
		n -= n % 2
		if n > 0 {
			if t.level != nil {
				t.level(b[:n])
			}
			t.mu.Lock()
			size := t.size
			t.mu.Unlock()
			h, e := wavHeader(size + int64(n))
			if e != nil {
				return e
			}
			if _, e = t.file.WriteAt(b[:n], 44+size); e != nil {
				return e
			}
			if _, e = t.file.WriteAt(h, 0); e != nil {
				return e
			}
			if e = t.file.Sync(); e != nil {
				return e
			}
			t.mu.Lock()
			t.size += int64(n)
			t.mu.Unlock()
			if size == 0 {
				close(t.ready)
			}
		}
		if readErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("audio source %s disconnected: %w", t.source.Label, readErr)
		}
	}
}

// Keep only the new interval when the recognizer sees one second of previous context.
func meetingWords(words []wordSegment, offsetMS, keepFromMS, keepToMS int64) []wordSegment {
	result := []wordSegment{}
	for _, w := range words {
		w.Offsets.From += offsetMS
		w.Offsets.To += offsetMS
		mid := (w.Offsets.From + w.Offsets.To) / 2
		if w.Offsets.To < w.Offsets.From || mid < keepFromMS || mid >= keepToMS {
			continue
		}
		result = append(result, w)
	}
	return result
}
func (s *Studio) checkpointMeeting(m *meetingSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	remote := alignSpeakers(append([]wordSegment(nil), m.remoteWords...), append([]speakerSegment(nil), m.speakers...))
	mic := alignSpeakers(append([]wordSegment(nil), m.micWords...), nil)
	for i := range mic {
		mic[i].Speaker = "You"
	}
	turns := append(mic, remote...)
	sort.SliceStable(turns, func(i, j int) bool { return turns[i].Start < turns[j].Start })
	text := speakerText(turns)
	data, err := json.MarshalIndent(struct {
		Started    time.Time     `json:"started"`
		Settings   Settings      `json:"settings"`
		Turns      []SpeakerTurn `json:"turns"`
		Transcript string        `json:"transcript"`
	}{m.started, m.config, turns, text}, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(m.folder, "transcript.pending.json")
	if err = os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		return err
	}
	return nil
}

func (s *Studio) runMeeting(captureCtx context.Context, m *meetingSession) {
	var runErr error
	defer func() { s.finishMeeting(m, runErr, false) }()
	sources, err := s.GetAudioSources()
	if err != nil {
		runErr = err
		return
	}
	mic, remote, err := resolveMeetingSources(sources, m.config)
	if err != nil {
		runErr = err
		return
	}
	// Watch the concrete devices chosen at start, even if the system defaults change.
	watchConfig := m.config
	watchConfig.MeetingMic = mic.ID
	watchConfig.MeetingAudio = remote.ID
	if captureCtx.Err() != nil {
		return
	}
	folder := filepath.Join(s.root, "meetings", m.started.Format("20060102-150405.000000000"))
	if err = os.MkdirAll(folder, 0700); err != nil {
		runErr = err
		return
	}
	m.folder = folder
	s.mu.Lock()
	s.meetingFolder = folder
	s.mu.Unlock()
	if err = saveMeetingRecording(m); err != nil {
		runErr = err
		return
	}
	tracks := []*meetingTrack{}
	for i, source := range []AudioSource{mic, remote} {
		name := []string{"microphone.wav", "meeting-audio.wav"}[i]
		f, e := os.OpenFile(filepath.Join(folder, name), os.O_CREATE|os.O_RDWR, 0600)
		if e != nil {
			runErr = e
			return
		}
		defer f.Close()
		h, _ := wavHeader(0)
		if _, e = f.Write(h); e != nil {
			runErr = e
			return
		}
		track := &meetingTrack{file: f, source: source, ready: make(chan struct{})}
		// The bubble meters the microphone, not unrelated remote playback.
		if i == 0 {
			track.level = func(pcm []byte) { s.emitAudioLevel(pcmLevel(pcm)) }
		}
		tracks = append(tracks, track)
	}
	captureErrors := make(chan error, 3)
	sourceDone := make(chan struct{})
	go func() {
		defer close(sourceDone)
		timer := time.NewTicker(2 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-timer.C:
			}
			checkCtx, cancel := context.WithTimeout(captureCtx, 4*time.Second)
			current, e := audioInventory(checkCtx)
			cancel()
			if captureCtx.Err() != nil {
				return
			}
			if e == nil {
				var selected AudioSource
				_, selected, e = resolveMeetingSources(current, watchConfig)
				if e == nil && selected.Device != remote.Device {
					e = errors.New("meeting app moved to another output; refresh and reselect its audio source")
				}
			}
			if e != nil {
				captureErrors <- e
				m.captureCancel()
				return
			}
		}
	}()
	defer func() { m.captureCancel(); <-sourceDone }()
	var captures sync.WaitGroup
	for _, t := range tracks {
		captures.Add(1)
		go func(t *meetingTrack) { defer captures.Done(); captureErrors <- t.capture(captureCtx, m.started) }(t)
	}
	defer func() { m.captureCancel(); captures.Wait() }()
	for _, track := range tracks {
		select {
		case <-track.ready:
		case e := <-captureErrors:
			runErr = e
			return
		case <-captureCtx.Done():
			return
		}
	}
	s.mu.Lock()
	if captureCtx.Err() == nil {
		s.status = "meeting"
	}
	s.mu.Unlock()
	s.ShowBubble()
	s.publish()

	select {
	case runErr = <-captureErrors:
	case <-captureCtx.Done():
	}
	m.captureCancel()
	captures.Wait()
	<-sourceDone
	for len(captureErrors) > 0 {
		if e := <-captureErrors; e != nil {
			runErr = e
		}
	}
	for i, t := range tracks {
		m.offsets[i] = t.offset
	}
	if err = saveMeetingRecording(m); err != nil {
		runErr = err
	}
}

func (s *Studio) processMeeting(ctx context.Context, m *meetingSession) {
	var runErr error
	defer func() { s.finishMeeting(m, runErr, true) }()
	bin, model, err := engine.Prepare(ctx, s.root, m.config.Model)
	if err != nil {
		runErr = err
		return
	}
	diarBin, diarModel, err := engine.PrepareLiveDiarization(ctx, s.root)
	if err != nil {
		runErr = err
		return
	}
	folder := m.folder
	log, err := os.OpenFile(filepath.Join(folder, "engine.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		runErr = err
		return
	}
	defer log.Close()
	processingCtx, cancelProcessing := context.WithCancel(ctx)
	defer cancelProcessing()
	diar := exec.CommandContext(processingCtx, diarBin, diarModel)
	diar.Stderr = log
	input, err := diar.StdinPipe()
	if err != nil {
		runErr = err
		return
	}
	output, err := diar.StdoutPipe()
	if err != nil {
		runErr = err
		return
	}
	if err = diar.Start(); err != nil {
		runErr = err
		return
	}
	ready := make(chan struct{})
	diarDone := make(chan error, 1)
	readerDone := make(chan struct{})
	defer func() {
		cancelProcessing()
		input.Close()
		if diar.Process != nil {
			diar.Process.Kill()
		}
		<-readerDone
	}()
	go func() {
		defer close(readerDone)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), 32*1024*1024)
		first := true
		var streamErr error
		for scanner.Scan() {
			var event struct {
				Ready    bool             `json:"ready"`
				Segments []speakerSegment `json:"segments"`
			}
			if e := json.Unmarshal(scanner.Bytes(), &event); e != nil {
				streamErr = e
				cancelProcessing()
				break
			}
			if event.Ready && first {
				close(ready)
				first = false
			}
			if event.Segments != nil {
				m.mu.Lock()
				for i := range event.Segments {
					event.Segments[i].Start += float64(m.remoteOffset) / 1000
					event.Segments[i].End += float64(m.remoteOffset) / 1000
				}
				m.speakers = event.Segments
				m.mu.Unlock()
				if e := s.checkpointMeeting(m); e != nil {
					streamErr = e
					cancelProcessing()
					break
				}
			}
		}
		if e := scanner.Err(); e != nil {
			streamErr = e
		}
		if e := diar.Wait(); e != nil && streamErr == nil {
			streamErr = fmt.Errorf("live speaker identification failed: %w (see meeting engine.log)", e)
		}
		diarDone <- streamErr
	}()
	select {
	case <-ready:
	case err = <-diarDone:
		runErr = fmt.Errorf("start live speaker identification: %v", err)
		return
	case <-ctx.Done():
		return
	}

	tracks := []*meetingTrack{}
	for i, name := range []string{"microphone.wav", "meeting-audio.wav"} {
		f, e := os.Open(filepath.Join(folder, name))
		if e != nil {
			runErr = e
			return
		}
		defer f.Close()
		size, e := meetingPCMSize(f)
		if e != nil {
			runErr = e
			return
		}
		tracks = append(tracks, &meetingTrack{file: f, size: size, finished: true, offset: m.offsets[i]})
	}
	consumed := [2]int64{}
	scratch, err := os.MkdirTemp(folder, "chunks-")
	if err != nil {
		runErr = err
		return
	}
	defer os.RemoveAll(scratch)
	for {
		allDone := true
		for i, t := range tracks {
			t.mu.Lock()
			size, finished := t.size, t.finished
			t.mu.Unlock()
			remaining := size - consumed[i]
			if !finished || remaining > 0 {
				allDone = false
			}
			if remaining < 28*32000 && !finished {
				continue
			}
			if remaining == 0 {
				continue
			}
			count := min(remaining, int64(28*32000))
			start := max(int64(0), consumed[i]-32000)
			pcm := make([]byte, consumed[i]+count-start)
			if _, err = t.file.ReadAt(pcm, 44+start); err != nil {
				runErr = err
				return
			}
			if i == 1 {
				m.mu.Lock()
				m.remoteOffset = t.offset
				m.mu.Unlock()
				if _, err = input.Write(pcm[consumed[i]-start:]); err != nil {
					runErr = err
					return
				}
			}
			sum := 0.0
			for n := 0; n < len(pcm); n += 2 {
				x := float64(int16(binary.LittleEndian.Uint16(pcm[n:]))) / 32768
				sum += x * x
			}
			if math.Sqrt(sum/float64(len(pcm)/2)) > m.config.Silence {
				wav := filepath.Join(scratch, "chunk.wav")
				prefix := filepath.Join(scratch, "chunk")
				if err = writeWAV(wav, pcm); err != nil {
					runErr = err
					return
				}
				name, args := engine.FileArgs(bin, model, m.config.Language, wav, prefix)
				filtered := []string{}
				for _, a := range args {
					if a != "-nt" {
						filtered = append(filtered, a)
					}
				}
				args = append(filtered, "-oj", "-ml", "1", "-sow")
				cmd := exec.CommandContext(processingCtx, name, args...)
				cmd.Stderr = log
				if err = cmd.Run(); err != nil {
					if processingCtx.Err() != nil && ctx.Err() == nil {
						runErr = <-diarDone
					} else {
						runErr = fmt.Errorf("meeting transcription failed: %w (see meeting engine.log)", err)
					}
					return
				}
				data, e := os.ReadFile(prefix + ".json")
				if e != nil {
					runErr = e
					return
				}
				var result struct {
					Transcription []wordSegment `json:"transcription"`
				}
				if e = json.Unmarshal(data, &result); e != nil {
					runErr = e
					return
				}
				words := meetingWords(result.Transcription, t.offset+start/32, t.offset+consumed[i]/32, t.offset+(consumed[i]+count)/32)
				m.mu.Lock()
				if i == 0 {
					m.micWords = append(m.micWords, words...)
				} else {
					m.remoteWords = append(m.remoteWords, words...)
				}
				m.mu.Unlock()
				if err = s.checkpointMeeting(m); err != nil {
					runErr = err
					return
				}
			}
			consumed[i] += count
		}

		if allDone {
			break
		}
		select {
		case runErr = <-diarDone:
			if runErr == nil {
				runErr = errors.New("speaker engine stopped unexpectedly")
			}
			return
		case <-processingCtx.Done():
			if ctx.Err() == nil {
				runErr = <-diarDone
			} else {
				runErr = ctx.Err()
			}
			return
		default:
		}
	}
	input.Close()
	select {
	case err = <-diarDone:
		runErr = err
	case <-ctx.Done():
		runErr = ctx.Err()
	}
	if runErr == nil {
		runErr = s.checkpointMeeting(m)
	}
}
