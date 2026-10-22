# Developing AFK

AFK uses Go, Wails v3, Svelte 5, Bun and shadcn-svelte/Bits UI. Wails uses the
system WebKit view. The CLI and desktop share the same model catalogue and native
engine integration; the desktop is a separate Go module.

## Build

Use Go 1.26 or newer, Bun, GTK3 and WebKitGTK 4.1 development packages. Native
engine preparation also needs Git, CMake, Ninja, a C++ compiler, pkg-config, SDL2
and CUDA. See the [Arch dependency list](../packaging/README.md#build-requirements).

```sh
make build
./bin/afk-desktop
```

`make build` starts with a clean `bin/`. `make cli` builds the console application;
`make desktop` builds the frontend and desktop executable. For browser-only UI
work, run `bun run dev` inside `frontend/`. Browser preview is labeled and does
not pretend to record audio or perform native operations.

Development desktop data defaults to `.afk/` under the project, resolved relative
to the executable. CLI development data defaults to `.afk/` under the current
working directory. Both accept `--data /absolute/path`. Installed packages use
XDG user data instead; see the [packaging guide](../packaging/README.md).

The development desktop builds CUDA engines for architecture 89. Package builds
use a portable configuration. The CLI also supports `--cuda-arch` and `--cpu`;
the desktop currently expects CUDA.

## Console transcription

```sh
./bin/afk models
./bin/afk prepare --model turbo-q5
./bin/afk listen --model turbo-q5 --language de
./bin/afk transcribe --model small-q5 --language en --file recording.wav
```

Stop live transcription with Ctrl+C. Use `en`, `de` or `auto` for language.
The CLI file input should be 16 kHz mono PCM WAV; the desktop performs conversion
with FFmpeg. Reuse the same recording when comparing models.

Useful controls include `--device N`, `--step 1000`, `--window 8000`,
`--silence-rms 0.003`, `--threads 8` and `--jobs 8`. Run a command with `--help`
for its options. The silence gate is an energy threshold, not a speech classifier.
The live decoder uses a rolling window: output revises, words can repeat at
boundaries, and a final audio-buffer flush is not guaranteed.

## Checks

```sh
make build
make test
go vet ./...
go -C desktop vet -tags gtk3 -buildvcs=false ./...
```

Build first in a fresh checkout so the desktop's embedded frontend assets exist.
Hardware integration tests are opt-in and require a desktop/audio/GPU session.
For the native smoke test, close AFK, prepare Small Q5 and supply a known speech
fixture:

```sh
make desktop
mkdir -p work
go -C desktop build -tags gtk3,smoke -buildvcs=false -o ../work/afk-smoke .
python3 scripts/test-desktop-audio.py --wav work/german.wav --expect Wurst
```

The smoke test uses temporary settings and a virtual audio sink; it checks native
bindings, clipboard behavior and bubble placement. Meeting test scripts under
`scripts/` use isolated virtual sources and prerecorded fixtures. These checks
verify functionality, not long-term transcription accuracy.

## Engine versions

- [whisper.cpp v1.9.5](https://github.com/ggml-org/whisper.cpp/tree/d1be6fde11ac6e0407606b4e42fe72d34add8037)
- [Whisper model revision](https://huggingface.co/ggerganov/whisper.cpp/tree/5359861c739e955e79d9a303bcbc70fb988958b1)
- [NeMo-Speech.cpp revision](https://github.com/NVIDIA/NeMo-Speech.cpp/tree/2bdfb03b38d5e3b0fa55aeb91a079a9f57385373)
- [Nemotron 3 Diarization model revision](https://huggingface.co/nvidia/Nemotron-3-Diarization/tree/f667ed73aee57d40cc39428eb768b4fd87a0a29e)

Model sizes and SHA-256 hashes live in `internal/engine`. Downloads are verified
before being made available. AFK patches Whisper's microphone example for its
silence gate and live output, without replacing the recognition model. Native
runtimes require no Python or PyTorch installation.

## CI and release builds

Only pushed version tags (`v*`) trigger CI. Normal branch pushes do not run
workflows, and there is no manual workflow trigger. The tag must match `VERSION`
(for example, `v0.3.0` for version `0.3.0`).

Each tag run validates the frontend and runs Go tests and vet, including the GTK
desktop module. After validation passes, it builds the Arch package, uploads the
package and checksum as a 30-day Actions artifact, and attaches them to a draft
release. Publishing the release remains a separate action.

The package job retains the native-engine cache. Reuse requires an exact match
for native sources and the Arch toolchain versions. A cache hit avoids rebuilding
those engines; its total runtime must be measured on a subsequent package run.
