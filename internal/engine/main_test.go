package engine

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestModelIntegrity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model")
	data := []byte("known model bytes")
	m := model{Size: int64(len(data)), SHA: fmt.Sprintf("%x", sha256.Sum256(data))}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verify(path, m); err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verify(path, m); err == nil {
		t.Fatal("accepted corrupted model of same size")
	}
	if err := os.WriteFile(path, data[:3], 0600); err != nil {
		t.Fatal(err)
	}
	if err := verify(path, m); err == nil {
		t.Fatal("accepted truncated model")
	}
}

func TestInferenceConfiguration(t *testing.T) {
	bin, args := inferenceArgs("listen", "/runtime/bin", "/model", "de", "", false, 4, 1000, 8000, -1)
	if filepath.Base(bin) != "whisper-stream" {
		t.Fatal(bin)
	}
	if slices.Contains(args, "-ng") || slices.Contains(args, "-tr") {
		t.Fatalf("GPU transcription unexpectedly disabled or translation enabled: %v", args)
	}
	for _, pair := range [][2]string{{"-l", "de"}, {"--step", "1000"}, {"--length", "8000"}, {"-mt", "0"}} {
		i := slices.Index(args, pair[0])
		if i < 0 || i+1 >= len(args) || args[i+1] != pair[1] {
			t.Fatalf("missing option %v: %v", pair, args)
		}
	}
	bin, args = inferenceArgs("transcribe", "/runtime/bin", "/model", "en", "speech.wav", true, 4, 1000, 8000, -1)
	if filepath.Base(bin) != "whisper-cli" || !slices.Contains(args, "-ng") || !slices.Contains(args, "speech.wav") {
		t.Fatal(bin, args)
	}
}

func TestInvalidInputDoesNotPrepareAnything(t *testing.T) {
	for _, args := range [][]string{{"listen", "--model", "unknown"}, {"listen", "--language", "fr"}, {"listen", "--step", "0"}, {"listen", "--window", "500"}, {"transcribe"}, {"listen", "--threads", "0"}, {"listen", "--silence-rms", "NaN"}, {"listen", "--silence-rms", "-1"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "untouched")
			args = append(args, "--data", root)
			if err := run(args); err == nil {
				t.Fatal("accepted invalid input")
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("invalid input modified cache")
			}
		})
	}
}
