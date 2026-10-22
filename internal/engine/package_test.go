package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func packagedRoot(t *testing.T) string {
	t.Helper()
	old := PackagedRuntimeRoot
	PackagedRuntimeRoot = t.TempDir()
	t.Cleanup(func() { PackagedRuntimeRoot = old })
	return PackagedRuntimeRoot
}

func TestInstalledDataDirectory(t *testing.T) {
	packagedRoot(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative-invalid")
	if got := DefaultDataDir(".afk"); got != filepath.Join(home, ".local/share/afk") {
		t.Fatal(got)
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	if got := DefaultDataDir(".afk"); got != filepath.Join(data, "afk") {
		t.Fatal(got)
	}
	PackagedRuntimeRoot = ""
	if got := DefaultDataDir("dev/.afk"); got != "dev/.afk" {
		t.Fatal(got)
	}
}

func TestInstalledRuntimesNeverInvokeCompiler(t *testing.T) {
	runtimeRoot := packagedRoot(t)
	data := t.TempDir()
	t.Setenv("PATH", "") // No compiler, git or CMake available to an end user.
	write := func(name, value string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(runtimeRoot, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), mode); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := prepareRuntime(context.Background(), data, false, "89", 1); err == nil {
		t.Fatal("missing installed engine accepted")
	}
	write("whisper/whisper-cli", "placeholder", 0755)
	write("whisper/whisper-stream", "placeholder", 0755)
	bin, err := prepareRuntime(context.Background(), data, false, "89", 1)
	if err != nil || bin != filepath.Join(runtimeRoot, "whisper") {
		t.Fatal(bin, err)
	}
	write("nemotron/nemo-speech", "placeholder", 0755)
	write("nemotron/libnemo_speech_asr_c.so", "placeholder", 0644)
	write("nemotron/afk-ready", DiarizationRevision, 0644)
	write("nemotron/afk-live-diar", "placeholder", 0755)
	_, model := DiarizationPaths(data)
	if err := os.MkdirAll(filepath.Dir(model), 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(model)
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(diarModel.Size); err != nil {
		t.Fatal(err)
	}
	file.Close()
	target, gotModel, err := PrepareLiveDiarization(context.Background(), data)
	if err != nil || target != filepath.Join(runtimeRoot, "nemotron/afk-live-diar") || gotModel != model {
		t.Fatal(target, gotModel, err)
	}
	if _, err = os.Stat(filepath.Join(runtimeRoot, "afk-live")); !os.IsNotExist(err) {
		t.Fatal("installed runtime tree was written")
	}
}
