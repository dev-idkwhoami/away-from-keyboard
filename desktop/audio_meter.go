package main

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"os/exec"
)

// A decibel-scaled RMS meter. Remove DC offset; silence remains exactly zero.
func pcmLevel(pcm []byte) float64 {
	n := len(pcm) / 2
	if n == 0 {
		return 0
	}
	var sum, squares float64
	for i := 0; i < n; i++ {
		x := float64(int16(binary.LittleEndian.Uint16(pcm[i*2:]))) / 32768
		sum += x
		squares += x * x
	}
	mean := sum / float64(n)
	rms := math.Sqrt(math.Max(0, squares/float64(n)-mean*mean))
	if rms <= .001 {
		return 0
	}
	return math.Min(1, math.Max(0, (20*math.Log10(rms)+60)/54))
}
func (s *Studio) emitAudioLevel(level float64) {
	if s.app != nil {
		s.app.Event.Emit("afk:audio-level", level)
	}
}

// Dictation's SDL/Pulse capture uses the default microphone (including PULSE_SOURCE).
// Meter that source independently of slow inference, without saving any audio.
func (s *Studio) startAudioMeter(parent context.Context) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer s.emitAudioLevel(0)
		_ = readAudioMeter(ctx, s.emitAudioLevel)
	}()
	return func() { cancel(); <-done }
}

func readAudioMeter(ctx context.Context, emit func(float64)) error {
	cmd := exec.CommandContext(ctx, "parec", "--raw", "--format=s16le", "--rate=16000", "--channels=1", "--latency-msec=50", "--client-name=AFK Level Meter")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	pcm := make([]byte, 1600)
	for {
		n, err := io.ReadFull(pipe, pcm)
		if n > 0 {
			emit(pcmLevel(pcm[:n]))
		}
		if err != nil {
			return err
		}
	}
}
