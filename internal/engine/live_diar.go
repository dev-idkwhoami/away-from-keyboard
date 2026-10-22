package engine

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed native/live_diar.cpp native/nemo_speech/*.h
var liveDiarSources embed.FS

func PrepareLiveDiarization(ctx context.Context, root string) (string, string, error) {
	if !DiarizationReady(root) {
		return "", "", fmt.Errorf("prepare Nemotron in Model Garage first")
	}
	bin, model := DiarizationPaths(root)
	dir, err := filepath.Abs(filepath.Dir(bin))
	if err != nil {
		return "", "", err
	}
	model, err = filepath.Abs(model)
	if err != nil {
		return "", "", err
	}
	if PackagedRuntimeRoot != "" {
		target := filepath.Join(dir, "afk-live-diar")
		return target, model, executable(target)
	}
	target, err := buildLiveHelper(ctx, dir)
	return target, model, err
}

func buildLiveHelper(ctx context.Context, dir string) (string, error) {
	hash := sha256.New()
	sourceDir := filepath.Join(filepath.Dir(dir), "afk-live")
	for _, name := range []string{"live_diar.cpp", "nemo_speech/asr.h", "nemo_speech/diar.h"} {
		contents, e := liveDiarSources.ReadFile("native/" + name)
		if e != nil {
			return "", e
		}
		hash.Write(contents)
		path := filepath.Join(sourceDir, name)
		if e = os.MkdirAll(filepath.Dir(path), 0700); e != nil {
			return "", e
		}
		if e = os.WriteFile(path, contents, 0600); e != nil {
			return "", e
		}
	}
	target := filepath.Join(dir, "afk-live-diar")
	stamp := fmt.Sprintf("%x", hash.Sum(nil))
	if data, e := os.ReadFile(target + ".stamp"); e == nil && string(data) == stamp {
		if _, e = os.Stat(target); e == nil {
			return target, nil
		}
	}
	cmd := exec.CommandContext(ctx, "c++", "-std=c++17", "-O2", "-I"+sourceDir, filepath.Join(sourceDir, "live_diar.cpp"), "-L"+dir, "-Wl,-rpath,$ORIGIN", "-lnemo_speech_asr_c", "-o", target+".tmp")
	if out, e := cmd.CombinedOutput(); e != nil {
		return "", fmt.Errorf("build live speaker helper: %w: %s", e, out)
	}
	if err := os.Rename(target+".tmp", target); err != nil {
		return "", err
	}
	if err := os.WriteFile(target+".stamp", []byte(stamp), 0600); err != nil {
		return "", err
	}
	return target, nil
}
