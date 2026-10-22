package main

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPCMLevel(t *testing.T) {
	wave := func(amplitude float64) []byte {
		b := make([]byte, 1600)
		for i := 0; i < 800; i++ {
			binary.LittleEndian.PutUint16(b[i*2:], uint16(int16(amplitude*32767*math.Sin(2*math.Pi*float64(i)/40))))
		}
		return b
	}
	if pcmLevel(nil) != 0 || pcmLevel(wave(0)) != 0 {
		t.Fatal("silence must be flat")
	}
	quiet, loud := pcmLevel(wave(.01)), pcmLevel(wave(.5))
	if !(quiet > 0 && loud > quiet && loud <= 1) {
		t.Fatal("volume does not scale", quiet, loud)
	}
	dc := make([]byte, 1600)
	for i := 0; i < 800; i++ {
		binary.LittleEndian.PutUint16(dc[i*2:], 1234)
	}
	if pcmLevel(dc) != 0 {
		t.Fatal("DC offset must not animate waveform")
	}
	if math.IsNaN(pcmLevel([]byte{1})) {
		t.Fatal("partial sample produced NaN")
	}
}

func TestLiveAudioMeter(t *testing.T) {
	sink := os.Getenv("AFK_METER_TEST_SINK")
	if sink == "" {
		t.Skip("requires isolated PulseAudio test sink")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	levels := make(chan float64, 100)
	done := make(chan error, 1)
	go func() {
		done <- readAudioMeter(ctx, func(level float64) {
			select {
			case levels <- level:
			default:
			}
		})
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("meter failed to stop")
		}
	}()
	select {
	case level := <-levels:
		if level != 0 {
			t.Fatal("isolated source not silent", level)
		}
	case <-ctx.Done():
		t.Fatal("meter did not start")
	}
	pcm := make([]byte, 32000)
	for i := 0; i < 16000; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(12000*math.Sin(2*math.Pi*440*float64(i)/16000))))
	}
	wav := filepath.Join(t.TempDir(), "tone.wav")
	if err := writeWAV(wav, pcm); err != nil {
		t.Fatal(err)
	}
	player := exec.CommandContext(ctx, "paplay", "--device="+sink, wav)
	if err := player.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = player.Process.Kill(); _ = player.Wait() }()
	heard := false
	for {
		select {
		case level := <-levels:
			if level > .5 {
				heard = true
			}
			if heard && level == 0 {
				return
			}
		case <-ctx.Done():
			t.Fatal("no loud-to-silent transition")
		}
	}
}
