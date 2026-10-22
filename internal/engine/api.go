package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// Run preserves the console interface while the desktop uses the same catalogue.
func Run(args []string) error { return run(args) }

type ModelInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Bytes        int64  `json:"bytes"`
	AutoLanguage bool   `json:"autoLanguage"`
	Installed    bool   `json:"installed"`
}

func Catalogue(root string) []ModelInfo {
	names := map[string]string{"turbo-q5": "Whisper Turbo · Q5", "turbo-q8": "Whisper Turbo · Q8", "large-v3-q5": "Whisper Large V3 · Q5", "small-q5": "Whisper Small · Q5"}
	out := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		st, err := os.Stat(filepath.Join(root, "models", m.File))
		out = append(out, ModelInfo{m.ID, names[m.ID], m.Size, m.AutoLanguage, err == nil && st.Size() == m.Size})
	}
	return out
}

func Prepare(ctx context.Context, root, id string) (bin, path string, err error) {
	m, err := lookup(id)
	if err != nil {
		return "", "", err
	}
	if err = os.MkdirAll(root, 0755); err != nil {
		return
	}
	lock, e := os.OpenFile(filepath.Join(root, "prepare.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return "", "", e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return "", "", fmt.Errorf("another process is preparing a model")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	bin, err = prepareRuntime(ctx, root, false, "89", min(runtime.NumCPU(), 8))
	if err != nil {
		return
	}
	path = filepath.Join(root, "models", m.File)
	err = prepareModel(ctx, path, m)
	return
}

func Remove(root, id string) error {
	m, err := lookup(id)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(root, "models", m.File))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func StreamArgs(bin, path, language string) (string, []string) {
	return inferenceArgs("listen", bin, path, language, "", false, min(runtime.NumCPU(), 8), 500, 8000, -1)
}

// FileArgs uses the same local engine and model catalogue as live dictation.
func FileArgs(bin, path, language, audio, output string) (string, []string) {
	name, args := inferenceArgs("transcribe", bin, path, language, audio, false, min(runtime.NumCPU(), 8), 0, 0, 0)
	return name, append(args, "-otxt", "-of", output)
}
