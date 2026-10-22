package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

const DiarizationID = "nemotron-3"
const DiarizationRevision = "2bdfb03b38d5e3b0fa55aeb91a079a9f57385373"
const diarModelRevision = "f667ed73aee57d40cc39428eb768b4fd87a0a29e"

var diarModel = model{DiarizationID, "Nemotron-3-Diarization.q8_0.gguf", "08456d9e22cd9a323c0364d98375f3746d6e68507ebb705cd46438c534c7a3a1", 107012128, false}

func DiarizationPaths(root string) (string, string) {
	bin := filepath.Join(root, "nemotron", DiarizationRevision, "build", "bin", "nemo-speech")
	if PackagedRuntimeRoot != "" {
		bin = filepath.Join(PackagedRuntimeRoot, "nemotron", "nemo-speech")
	}
	return bin, filepath.Join(root, "models", diarModel.File)
}
func DiarizationReady(root string) bool {
	bin, path := DiarizationPaths(root)
	st, err := os.Stat(path)
	if err != nil || st.Size() != diarModel.Size {
		return false
	}
	st, err = os.Stat(bin)
	if err != nil || st.Mode()&0111 == 0 {
		return false
	}
	if st, err = os.Stat(filepath.Join(filepath.Dir(bin), "libnemo_speech_asr_c.so")); err != nil || st.Size() == 0 {
		return false
	}
	stamp, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "afk-ready"))
	return err == nil && string(stamp) == DiarizationRevision
}
func PrepareDiarization(ctx context.Context, root string) (string, string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	if err = os.MkdirAll(root, 0755); err != nil {
		return "", "", err
	}
	lock, err := os.OpenFile(filepath.Join(root, "prepare.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return "", "", err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return "", "", errors.New("another model preparation is running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	bin, path := DiarizationPaths(root)
	if err = prepareModelURL(ctx, path, diarModel, "https://huggingface.co/nvidia/Nemotron-3-Diarization/resolve/"+diarModelRevision+"/"+diarModel.File); err != nil {
		return "", "", err
	}
	if DiarizationReady(root) {
		return bin, path, nil
	}
	if PackagedRuntimeRoot != "" {
		return "", "", errors.New("installed Nemotron runtime is incomplete; reinstall AFK")
	}
	bin, err = buildDiarization(ctx, root, "89", min(runtime.NumCPU(), 6), false)
	return bin, path, err
}

func buildDiarization(ctx context.Context, root, arch string, jobs int, portable bool) (string, error) {
	base := filepath.Join(root, "nemotron", DiarizationRevision)
	source := filepath.Join(base, "source")
	build := filepath.Join(base, "build")
	if portable {
		build += "-package-" + arch
	}
	bin := filepath.Join(build, "bin", "nemo-speech")
	var err error
	if err = os.MkdirAll(source, 0755); err != nil {
		return "", err
	}
	logPath := filepath.Join(root, "nemotron-prepare.log")
	if portable {
		fmt.Fprintf(os.Stderr, "Building Nemotron runtime (details: %s)\n", logPath)
	}
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer log.Close()
	run := func(name string, args ...string) error {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return nil
			}
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		cmd.WaitDelay = 5 * time.Second
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s failed: %w (details: %s)", name, err, logPath)
		}
		return nil
	}
	commands := [][]string{
		{"git", "-C", source, "init"},
		{"git", "-C", source, "fetch", "--depth", "1", "https://github.com/NVIDIA/NeMo-Speech.cpp.git", DiarizationRevision},
		{"git", "-C", source, "checkout", "--detach", "FETCH_HEAD"},
		{"git", "-C", source, "submodule", "update", "--init", "--depth", "1", "llama.cpp"},
		{"bash", filepath.Join(source, "scripts", "build_sentencepiece_static.sh")},
		{"cmake", "-S", source, "-B", build, "-G", "Ninja", "-DCMAKE_BUILD_TYPE=Release", "-DNEMO_SPEECH_BUILD_ASR=OFF", "-DNEMO_SPEECH_BUILD_TTS=OFF", "-DNEMO_SPEECH_BUILD_MIC_CAPTURE=OFF", "-DGGML_CUDA=ON", "-DCMAKE_CUDA_COMPILER=/opt/cuda/bin/nvcc", "-DCMAKE_CUDA_ARCHITECTURES=" + arch},
		{"cmake", "--build", build, "--target", "nemo-speech", "nemo_speech_asr_c", "-j", strconv.Itoa(jobs)},
	}
	if portable {
		commands[5] = append(commands[5], "-DGGML_NATIVE=OFF", "-DGGML_CUDA_NCCL=OFF", "-DCMAKE_BUILD_RPATH_USE_ORIGIN=ON")
	}
	for _, cmd := range commands {
		if err = run(cmd[0], cmd[1:]...); err != nil {
			return "", err
		}
	}
	if err = os.WriteFile(filepath.Join(filepath.Dir(bin), "afk-ready"), []byte(DiarizationRevision), 0600); err != nil {
		return "", err
	}
	return bin, nil
}
func RemoveDiarization(root string) error {
	_, path := DiarizationPaths(root)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
