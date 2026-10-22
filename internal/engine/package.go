package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Set only for installed builds; development builds keep their local cache.
var PackagedRuntimeRoot string

func DefaultDataDir(development string) string {
	if PackagedRuntimeRoot == "" {
		return development
	}
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "afk")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("locate AFK user data directory: %w", err))
	}
	return filepath.Join(home, ".local", "share", "afk")
}

func executable(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("installed AFK runtime: %w; reinstall AFK", err)
	}
	if !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return fmt.Errorf("installed AFK runtime is not executable: %s", path)
	}
	return nil
}

// BuildPackageRuntimes builds native engines without downloading model weights.
// Architecture 75 includes PTX, allowing newer NVIDIA GPUs to JIT it at first use.
func BuildPackageRuntimes(ctx context.Context, root, arch string, jobs int) (string, string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	if jobs < 1 {
		return "", "", fmt.Errorf("jobs must be positive")
	}
	whisper, err := buildRuntime(ctx, root, false, arch, jobs, true)
	if err != nil {
		return "", "", err
	}
	nemo, err := buildDiarization(ctx, root, arch, jobs, true)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Dir(nemo)
	if _, err = buildLiveHelper(ctx, dir); err != nil {
		return "", "", err
	}
	return whisper, dir, nil
}
