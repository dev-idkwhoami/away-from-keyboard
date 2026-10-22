// AFK is a small local transcription test harness. Inference and microphone
// capture run in a pinned native whisper.cpp executable; no Python is needed.
package engine

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const runtimeRevision = "d1be6fde11ac6e0407606b4e42fe72d34add8037" // whisper.cpp v1.9.5
const modelRevision = "5359861c739e955e79d9a303bcbc70fb988958b1"
const buildRecipe = runtimeRevision + ":afk-desktop-events-3"

//go:embed native/terminal.h
var terminalHeader []byte

type model struct {
	ID, File, SHA string
	Size          int64
	AutoLanguage  bool
}

var models = []model{
	{"turbo-q5", "ggml-large-v3-turbo-q5_0.bin", "394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2", 574041195, true},
	{"turbo-q8", "ggml-large-v3-turbo-q8_0.bin", "317eb69c11673c9de1e1f0d459b253999804ec71ac4c23c17ecf5fbe24e259a1", 874188075, true},
	{"large-v3-q5", "ggml-large-v3-q5_0.bin", "d75795ecff3f83b5faa89d1900604ad8c780abd5739fae406de19f23ecd98ad1", 1081140203, true},
	{"small-q5", "ggml-small-q5_1.bin", "ae85e4a935d7a567bd102fe55afc16bb595bdb618e11b2fc7591bc08120411bb", 190085487, true},
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		usage()
		return nil
	}
	if args[0] == "models" {
		for _, m := range models {
			fmt.Printf("%-14s %7.0f MB  languages=en,de  auto=%t  %s\n", m.ID, float64(m.Size)/1e6, m.AutoLanguage, m.File)
		}
		return nil
	}
	command := args[0]
	if command != "prepare" && command != "listen" && command != "transcribe" {
		return fmt.Errorf("unknown command %q; run afk help", command)
	}
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	id := f.String("model", "turbo-q5", "model ID (afk models)")
	data := f.String("data", DefaultDataDir(".afk"), "cache directory, relative to current directory")
	lang := f.String("language", "auto", "auto, en, or de (fixed language is more reliable for short phrases)")
	cpu := f.Bool("cpu", false, "explicit CPU backend instead of CUDA")
	arch := f.String("cuda-arch", "89", "CUDA architecture; 89 = RTX 4080 SUPER")
	jobs := f.Int("jobs", min(runtime.NumCPU(), 8), "parallel build jobs")
	threads := f.Int("threads", min(runtime.NumCPU(), 8), "inference CPU threads")
	step := f.Int("step", 1000, "live update interval in milliseconds")
	window := f.Int("window", 8000, "rolling audio window in milliseconds")
	device := f.Int("device", -1, "SDL microphone index; -1 = default (inventory printed at startup)")
	silence := f.Float64("silence-rms", 0.003, "skip live audio below this RMS level (0 disables; lower for a quiet microphone)")
	audio := f.String("file", "", "audio file for transcribe, preferably 16 kHz mono WAV")
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(f.Args(), " "))
	}
	m, err := lookup(*id)
	if err != nil {
		return err
	}
	if *lang != "auto" && *lang != "en" && *lang != "de" {
		return errors.New("language must be auto, en, or de")
	}
	if *lang == "auto" && !m.AutoLanguage {
		return fmt.Errorf("model %s does not support automatic language detection", m.ID)
	}
	if *jobs < 1 || *threads < 1 {
		return errors.New("jobs and threads must be positive")
	}
	if math.IsNaN(*silence) || math.IsInf(*silence, 0) || *silence < 0 || *silence > 1 {
		return errors.New("silence-rms must be between 0 and 1")
	}
	if *step < 250 || *window < 2**step || *window > 30000 {
		return errors.New("require step >= 250 ms and 2*step <= window <= 30000 ms")
	}
	if command == "transcribe" {
		if *audio == "" {
			return errors.New("transcribe requires --file")
		}
		if _, err := os.Stat(*audio); err != nil {
			return err
		}
	}
	root, err := filepath.Abs(*data)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Serialize preparation so two invocations cannot corrupt a partial download
	// or the native build. The lock is released before inference.
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(root, "prepare.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another AFK process is preparing models; try again when it finishes")
	}
	bin, err := prepareRuntime(ctx, root, *cpu, *arch, *jobs)
	if err != nil {
		return err
	}
	modelPath := filepath.Join(root, "models", m.File)
	if err := prepareModel(ctx, modelPath, m); err != nil {
		return err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		return err
	}
	if command == "prepare" {
		fmt.Printf("Ready: %s\nRuntime: %s\nModel: %s\n", m.ID, bin, modelPath)
		return nil
	}
	if !*cpu {
		out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader").CombinedOutput()
		if err != nil {
			return fmt.Errorf("CUDA GPU is inaccessible (use --cpu only if intended): %s", out)
		}
		fmt.Fprintf(os.Stderr, "GPU: %s", out)
	}
	native, nativeArgs := inferenceArgs(command, bin, modelPath, *lang, *audio, *cpu, *threads, *step, *window, *device)
	fmt.Fprintf(os.Stderr, "AFK | %s | language=%s\n", m.ID, *lang)
	if command == "listen" {
		fmt.Fprintln(os.Stderr, "Listening to your microphone. Ctrl+C stops. Text is provisional and revises within each window.")
	}
	// Replace the launcher: the native process receives terminal signals directly,
	// keeps one model loaded, and cannot be left behind as an orphan worker.
	stop()
	if err := os.Setenv("AFK_SILENCE_RMS", strconv.FormatFloat(*silence, 'g', -1, 64)); err != nil {
		return err
	}
	return syscall.Exec(native, append([]string{native}, nativeArgs...), os.Environ())
}

func usage() {
	fmt.Println(`AFK — Away From Keyboard, local transcription experiments

  afk models
  afk prepare [--model turbo-q5] [--cpu]
  afk listen [--model turbo-q5] [--language auto|en|de]
  afk transcribe --file recording.wav [--model turbo-q5] [--language de]

listen/transcribe automatically prepare missing runtime and models.
Use <command> --help for options. Once prepared, inference is local and offline.`)
	if PackagedRuntimeRoot != "" {
		fmt.Printf("Models and state: %s\nEngines are preinstalled; only model downloads need internet.\nCUDA is the default; --cpu disables GPU inference.\n", DefaultDataDir(".afk"))
	} else {
		fmt.Println("Cache defaults to .afk in the current directory.\nCUDA is the default; --cpu selects an independent CPU build.\nFirst setup needs internet, Git, CMake, C/C++, SDL2 development files and CUDA.")
	}
}

func lookup(id string) (model, error) {
	for _, m := range models {
		if m.ID == id {
			return m, nil
		}
	}
	return model{}, fmt.Errorf("unknown model %q; run afk models", id)
}

func inferenceArgs(command, bin, path, lang, audio string, cpu bool, threads, step, window, device int) (string, []string) {
	a := []string{"-m", path, "-l", lang, "-t", strconv.Itoa(threads), "-nf"}
	if cpu {
		a = append(a, "-ng")
	}
	if command == "transcribe" {
		return filepath.Join(bin, "whisper-cli"), append(a, "-f", audio, "-nt")
	}
	a = append(a, "--step", strconv.Itoa(step), "--length", strconv.Itoa(window), "--keep", "300", "-c", strconv.Itoa(device), "-mt", "0")
	return filepath.Join(bin, "whisper-stream"), a
}

func runTool(ctx context.Context, name string, args ...string) error {
	fmt.Fprintln(os.Stderr, "+", name, strings.Join(args, " "))
	c := exec.CommandContext(ctx, name, args...)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func prepareRuntime(ctx context.Context, root string, cpu bool, arch string, jobs int) (string, error) {
	if PackagedRuntimeRoot != "" {
		bin := filepath.Join(PackagedRuntimeRoot, "whisper")
		for _, name := range []string{"whisper-cli", "whisper-stream"} {
			if err := executable(filepath.Join(bin, name)); err != nil {
				return "", err
			}
		}
		return bin, nil
	}
	return buildRuntime(ctx, root, cpu, arch, jobs, false)
}

func buildRuntime(ctx context.Context, root string, cpu bool, arch string, jobs int, portable bool) (string, error) {
	if _, err := strconv.Atoi(arch); err != nil {
		return "", errors.New("cuda-arch must be numeric, e.g. 89")
	}
	flavor := "cuda-" + arch
	if cpu {
		flavor = "cpu"
	}
	if portable {
		flavor = "package-" + flavor
	}
	source := filepath.Join(root, "runtime", runtimeRevision)
	build := filepath.Join(source, "build-"+flavor)
	bin := filepath.Join(build, "bin")
	marker := filepath.Join(build, "afk-ready")
	if ready, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(ready)) == buildRecipe {
		if _, err := os.Stat(filepath.Join(bin, "whisper-stream")); err == nil {
			if _, err := os.Stat(filepath.Join(bin, "whisper-cli")); err == nil {
				return bin, nil
			}
		}
	}
	for _, name := range []string{"git", "cmake", "pkg-config"} {
		if _, err := exec.LookPath(name); err != nil {
			return "", fmt.Errorf("missing prerequisite %s", name)
		}
	}
	if err := exec.CommandContext(ctx, "pkg-config", "--exists", "sdl2").Run(); err != nil {
		return "", errors.New("SDL2 development files are required (pkg-config sdl2 failed)")
	}
	if err := os.MkdirAll(source, 0755); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(source, ".git")); os.IsNotExist(err) {
		if err := runTool(ctx, "git", "init", source); err != nil {
			return "", err
		}
	}
	if err := exec.CommandContext(ctx, "git", "-C", source, "cat-file", "-e", runtimeRevision+"^{commit}").Run(); err != nil {
		if err := runTool(ctx, "git", "-C", source, "fetch", "--depth=1", "https://github.com/ggml-org/whisper.cpp.git", runtimeRevision); err != nil {
			return "", err
		}
	}
	if err := runTool(ctx, "git", "-C", source, "checkout", "--detach", runtimeRevision); err != nil {
		return "", err
	}
	// Enabling SDL also enables upstream's unrelated talk-llama example, which
	// downloads a full llama.cpp repository during configuration. We only need
	// microphone ASR: omit that one example without changing inference code.
	examples := filepath.Join(source, "examples", "CMakeLists.txt")
	cmakeFile, err := os.ReadFile(examples)
	if err != nil {
		return "", err
	}
	cmakeFile = []byte(strings.ReplaceAll(string(cmakeFile), "add_subdirectory(talk-llama)", "# AFK: omit unrelated talk-llama example"))
	if err := os.WriteFile(examples, cmakeFile, 0644); err != nil {
		return "", err
	}
	streamPath := filepath.Join(source, "examples", "stream", "stream.cpp")
	stream, err := os.ReadFile(streamPath)
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(stream), "// AFK silence gate") {
		needle := "        // run the inference"
		if strings.Count(string(stream), needle) != 1 {
			return "", errors.New("unexpected upstream stream source: cannot install silence gate")
		}
		gate := `        // AFK silence gate: skip quiet new audio, not the accumulated window.
        // This is an energy gate, not a speech classifier.
        const char * gate_env = std::getenv("AFK_SILENCE_RMS");
        const double gate_level = gate_env ? std::atof(gate_env) : 0.003;
        if (!pcmf32_new.empty() && gate_level > 0.0) {
            double sum = 0.0, squares = 0.0;
            for (float sample : pcmf32_new) { sum += sample; squares += sample * sample; }
            const double mean = sum / pcmf32_new.size();
            const double rms = std::sqrt(std::max(0.0, squares / pcmf32_new.size() - mean * mean));
            if (rms < gate_level) { continue; }
        }

`
		stream = []byte(strings.Replace(string(stream), needle, gate+needle, 1))
		stream = append([]byte("#include <cmath>\n#include <cstdlib>\n"), stream...)
		if err := os.WriteFile(streamPath, stream, 0644); err != nil {
			return "", err
		}
	}
	if err := patchTerminal(source, streamPath); err != nil {
		return "", err
	}
	a := []string{"-S", source, "-B", build, "-DCMAKE_BUILD_TYPE=Release", "-DWHISPER_BUILD_TESTS=OFF", "-DWHISPER_SDL2=ON", "-DWHISPER_BUILD_SERVER=OFF"}
	if portable {
		a = append(a, "-DGGML_NATIVE=OFF", "-DGGML_CUDA_NCCL=OFF", "-DCMAKE_BUILD_RPATH_USE_ORIGIN=ON")
	}
	if cpu {
		a = append(a, "-DGGML_CUDA=OFF")
	} else {
		nvcc, err := exec.LookPath("nvcc")
		if err != nil {
			nvcc = "/opt/cuda/bin/nvcc"
			if _, err := os.Stat(nvcc); err != nil {
				return "", errors.New("CUDA compiler missing; install CUDA or explicitly select --cpu")
			}
		}
		a = append(a, "-DGGML_CUDA=ON", "-DCMAKE_CUDA_COMPILER="+nvcc, "-DCMAKE_CUDA_ARCHITECTURES="+arch)
	}
	if err := runTool(ctx, "cmake", a...); err != nil {
		return "", err
	}
	if err := runTool(ctx, "cmake", "--build", build, "--target", "whisper-stream", "whisper-cli", "-j", strconv.Itoa(jobs)); err != nil {
		return "", err
	}
	if err := os.WriteFile(marker, []byte(buildRecipe+"\n"), 0644); err != nil {
		return "", err
	}
	return bin, nil
}

func verify(path string, m model) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if st.Size() != m.Size {
		return fmt.Errorf("wrong model size: got %d, expected %d", st.Size(), m.Size)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if fmt.Sprintf("%x", h.Sum(nil)) != m.SHA {
		return errors.New("model SHA-256 mismatch")
	}
	return nil
}

func prepareModel(ctx context.Context, path string, m model) error {
	return prepareModelURL(ctx, path, m, "https://huggingface.co/ggerganov/whisper.cpp/resolve/"+modelRevision+"/"+m.File)
}

func prepareModelURL(ctx context.Context, path string, m model, u string) error {
	if _, err := os.Stat(path); err == nil {
		if err := verify(path, m); err != nil {
			return fmt.Errorf("%s: %w; remove the damaged file and prepare again", path, err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Downloading %s (%.0f MB)…\n", m.ID, float64(m.Size)/1e6)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("model download: HTTP %d", res.StatusCode)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	p := &progress{total: m.Size, last: time.Now()}
	_, copyErr := io.Copy(f, io.TeeReader(io.LimitReader(res.Body, m.Size+1), p))
	closeErr := f.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := verify(tmp, m); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Model downloaded and SHA-256 verified.")
	return nil
}

type progress struct {
	n, total int64
	last     time.Time
}

func (p *progress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if time.Since(p.last) > 3*time.Second {
		fmt.Fprintf(os.Stderr, "  %.0f / %.0f MB\n", float64(p.n)/1e6, float64(p.total)/1e6)
		p.last = time.Now()
	}
	return len(b), nil
}
