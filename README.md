# AFK — Away From Keyboard

Local English/German dictation for CachyOS: a Wails v3 desktop app and a Go
CLI sharing the same verified whisper.cpp/CUDA engine. The desktop uses the
system WebKit view, Svelte 5, Bun, and shadcn-svelte/Bits UI controls.

## Desktop

```sh
make                  # clean bin/, build both executables
./bin/afk-desktop
```

Requires Go 1.26+, Bun, `gtk3`, `webkit2gtk-4.1`, and the native inference
requirements below. `make cli` builds just the CLI without WebKit dependencies;
`make desktop` builds the frontend and desktop executable. The desktop is a
separate Go module so CLI-only users do not need the GUI toolchain.

- **Studio:** start/stop dictation, choose model/language, view the latest text.
- **Models:** download, select, or remove the four supported models. Downloads
  verify SHA-256; preparation is shown as indeterminate progress.
- **History:** optionally retain the latest 200 transcripts; copy again or delete.
- **Settings:** light/dark/system theme, bubble mode/position, history, and silence gate.
- **Bubble:** transparent, frameless window; click the synthetic waveform to
  expand/collapse live text. Its mic button starts/stops, AFK mark opens Studio,
  and × hides it. Quit is available in Studio and the expanded bubble.

Stopping copies the latest transcript to the clipboard. Closing Studio or the bubble hides that window to the system tray; it does not
quit AFK or stop an active recording. Click the tray icon or choose Open
to reopen Studio. Choose Close from the tray menu to exit.
AFK keeps listening for the global shortcut while hidden.
The bubble appears only when the microphone is ready and listening, and hides
on stop or failure; use the power button to quit AFK. The waveform is an
animated recording indicator, not an audio-level meter.

Settings → One shortcut. Anywhere configures the global dictation toggle.
The default is `Ctrl+Shift+Space`. Choose modifiers and a key, then Apply;
Disable releases it. AFK registers with KDE KGlobalAccel, so the shortcut works
across Wayland and XWayland apps while AFK is running, including with its windows
hidden. Conflicting shortcuts are rejected and your previous shortcut stays active.
The choice is saved locally and registered again on launch; quitting releases it.
Other desktop environments currently need their own shortcut integration.

The `afk-desktop --toggle` command remains available for external launchers.
AFK uses GTK's X11
backend (XWayland within KDE Wayland) when DISPLAY is available, because normal
Wayland windows cannot request absolute placement. X11/XWayland is required for
reliable automatic placement.

Settings → Bubble position offers all nine screen anchors, defaulting to top
middle. The bubble uses the primary monitor's work area with a 20 px edge inset,
and reapplies the anchor on show, mode/size changes, and position changes. Old
settings files automatically inherit the top-middle default.

Models/runtime are shared with the CLI in `.afk/`. Desktop settings/history are
stored atomically in `.afk/desktop-state.json` with owner-only permissions;
recording diagnostics go to `.afk/desktop-engine.log`. No audio files are saved.
Use `--data /absolute/path` to override the data directory. The default resolves
relative to the executable's parent project directory, independent of shell cwd.

The streaming decoder still has the CLI's provisional-text/boundary limitations,
including no guaranteed final audio-buffer flush. Wait briefly after your last
word before stopping. Models are released when a session ends and loaded again
on the next start. The desktop currently targets NVIDIA CUDA architecture 89
and the default microphone, matching the tested machine.

For frontend-only design work: `cd frontend && bun run dev`. Browser preview
labels itself and does not simulate successful recording/download/clipboard work.

## CLI

From this directory:

```sh
make cli
./bin/afk models
./bin/afk prepare --model turbo-q5
./bin/afk listen --model turbo-q5 --language de
```

Speak into the default microphone. Press **Ctrl+C** to stop. Use `--language en`
for English or `--language auto` for automatic detection. Explicit language is
usually more reliable for short phrases. Auto detection is not a promise of
perfect code switching within a sentence.

The model stays loaded for the entire session. Live output stays on one row:
the newest text remains visible and older text drops off the left. The provisional
tail revises as more audio arrives. Output is clipped to the terminal width, with
UTF-8 characters kept intact; periodic audio-window resets do not add new lines.
These are provisional hypotheses, not independent sentences to concatenate.
This first experiment uses whisper.cpp's rolling-window
microphone example, not a production streaming decoder. It can repeat boundary
words or lose words when inference falls behind. A basic energy gate suppresses
quiet audio; it is not a speech classifier, so background sounds can still
produce hallucinations.
There is no final-buffer flush guarantee on Ctrl+C yet.

## Compare models one at a time

```sh
./bin/afk listen --model turbo-q8 --language de
./bin/afk listen --model large-v3-q5 --language de
./bin/afk listen --model small-q5 --language de
```

Stop each session before starting another. Missing models prepare automatically.

| ID | Model | Download | English / German | Auto language |
|---|---|---:|---|---|
| `turbo-q5` | Whisper large-v3-turbo, Q5_0 | 574 MB | Yes | Yes |
| `turbo-q8` | Whisper large-v3-turbo, Q8_0 | 874 MB | Yes | Yes |
| `large-v3-q5` | Whisper large-v3, Q5_0 | 1,081 MB | Yes | Yes |
| `small-q5` | Whisper small multilingual, Q5_1 | 190 MB | Yes | Yes |

This is AFK's current supported model list. `afk models` shows language capability
alongside download size. All four use multilingual Whisper checkpoints, including
Small (not `small.en`). Auto detection is supported but not restricted to English
and German, and can guess incorrectly on short/noisy speech. Fixed `en`/`de` is
available for those cases. Quantization does not remove language detection.

For the future UI, the model-management area should expose download/remove/select,
download progress and size, and each model's language capabilities. The catalogue's
`AutoLanguage` field controls eligibility for the Auto option; runtime validation
rejects Auto for any future model without that capability. The UI is not built yet.

Only Whisper-family models are implemented in this first harness. Parakeet,
Cohere, and Qwen require additional runtime adapters; arbitrary weights cannot
be loaded into this backend.

For fair comparisons, reuse the same recording:

```sh
./bin/afk transcribe --model turbo-q5 --language de --file recording.wav
./bin/afk transcribe --model turbo-q8 --language de --file recording.wav
```

Use a 16 kHz mono PCM WAV. Native logs include inference timing and GPU backend
details. Compare transcript mistakes as well as speed; live refresh frequency
alone is not an accuracy measurement.

## Requirements and controls

- Linux, Go 1.24+, Git, CMake, a C/C++ compiler, pkg-config and SDL2 development files.
- CUDA toolkit and a working NVIDIA driver for the default CUDA backend.
- CUDA architecture defaults to **89** for this project's RTX 4080 SUPER.
- Internet for first preparation only. Cached models work offline.
- Cache/build/model files are in `.afk/` relative to the working directory;
  `--data /absolute/path` selects another location.
- `--cpu` explicitly selects a separately built CPU runtime.
- `--device N` selects an SDL capture index; the native runtime prints devices
  on startup. Default `-1` uses the system default microphone.
- `--step 1000 --window 8000` controls update interval and rolling context in ms.
- `--silence-rms 0.003` skips quiet input. Lower it (e.g. `0.001`) if quiet speech
  is missed; `0` disables the gate for comparison. This is a linear amplitude
  threshold, not a probability.
- `--threads 8` controls inference CPU threads; `--jobs 8` controls build jobs.
- The CUDA build prints its backend at startup. Confirm `using CUDA0 backend`;
  `nvidia-smi` preflight alone is not proof of GPU inference.

The CLI records only while `listen` is running. Audio is processed in memory and
not saved by the launcher; output remains in your terminal. Desktop recording
runs only between start and stop.

## Reproducibility

Runtime: [whisper.cpp v1.9.5](https://github.com/ggml-org/whisper.cpp/tree/d1be6fde11ac6e0407606b4e42fe72d34add8037).
Models: [pinned upstream GGML files](https://huggingface.co/ggerganov/whisper.cpp/tree/5359861c739e955e79d9a303bcbc70fb988958b1).
Model sizes and SHA-256 hashes are embedded in the Go model catalogue. A damaged
cache is rejected; downloads use temporary files and become visible only after
verification. Native source and artifacts remain in `.afk/runtime/`.
The build omits the unrelated `talk-llama` example, adds the configurable energy
gate, and replaces the stream example's terminal printing with a bounded single-row
view. Routine auto-detection logs are suppressed while listening; other native
diagnostics remain available. It does not change the recognition model or decoder.

```sh
make test
go vet ./...
```

## Local verification (2026-10-22)

Tested on RTX 4080 SUPER, CUDA architecture 89, KDE/Wayland:

- Go race tests and vet passed.
- Single-row renderer regression tests passed with narrow/resized pseudo-terminals,
  German umlauts, combining accents, and wide characters. A ten-second Small Q5
  microphone run produced 10 row refreshes, only the startup/shutdown newlines,
  no repeated auto-language logs, and no reported dropped audio.
- Turbo Q5 and Small Q5 downloaded and passed size/SHA-256 verification.
- Turbo Q5 transcribed the bundled 11-second JFK English sample correctly;
  native total time was 576 ms, including 423 ms model load.
- On a [public German test clip](https://huggingface.co/csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8/blob/main/test_wavs/de.wav),
  Turbo Q5 produced “Alles hat ein Ende, nur die Wurst hat zwei.” Small Q5
  produced “Alles hat ein Ende, nur die Worst hat zwei.” This tiny fixture is a
  functional check, not an accuracy benchmark.
- An eight-second microphone check opened the default 16 kHz mono capture
  device, ran inference on CUDA0, printed live hypotheses, and stopped on SIGINT.
- The energy gate was added after the initial microphone check produced short
  fragments in quiet audio. A second check confirmed live output still works;
  this does not establish reliable noise rejection or quiet-speech recall.

The user also tested Small Q5 and Large V3 Q5 interactively. Turbo Q8 and the
CPU backend have not been evaluated here. Longer bilingual tests are still
needed to compare accuracy and tune the silence gate.

## Desktop verification

- `make build`, `make test`, and Go vet passed for both modules.
- Browser checks covered both themes, language selection, the history switch,
  bubble expansion, model/history navigation, and the 900 px minimum width.
- A standalone native smoke build checked both WebKit windows, real Go bindings,
  cross-window theme events, transparent bubble CSS, and clipboard read-back.
- An isolated PulseAudio monitor supplied the German fixture to the CUDA worker.
  The result was “Alles hat ein Ende, nur die Wurst hat zwei.” Clipboard and
  persisted history matched. The single-instance `--toggle` command was used
  to start and stop the final test.

The native smoke build is excluded from normal builds. To rerun it, close AFK,
prepare Small Q5, build the desktop assets, and supply a known speech fixture:

```sh
make desktop
mkdir -p work
go -C desktop build -tags gtk3,smoke -buildvcs=false -o ../work/afk-smoke .
python3 scripts/test-desktop-audio.py --wav work/german.wav --expect Wurst
```

This requires `pactl`, `paplay`, and a desktop session. It creates/removes its own
virtual audio sink, uses temporary settings/history, and restores the prior text
clipboard when available. Supply your own fixture or the linked German test clip.
Native placement regression checks verify all nine anchors and hide/show placement
on KDE through XWayland, plus compact/expanded/collapsed frameless sizes. The smoke
check does not establish long-term dictation accuracy.

## Audio file transcription

The tray menu contains Open, Transcribe file, Choose Model, and Close.
Quick switch lists the supported models, checks the selected one, and enables
only installed models while idle. Close exits; window close buttons only hide.

Choose Transcribe file in the tray or Choose file in Studio. FFmpeg converts
the first audio track to 16 kHz mono PCM before the selected local Whisper model
transcribes it. WAV, MP3, M4A, FLAC, Ogg and other FFmpeg-readable formats work.
The source file is untouched. The UI shows conversion/preparation/transcription
stages and offers Cancel import. One audio operation runs at a time. The bubble
stays hidden during imports. Completed text appears in Studio for copying and
is saved in History when enabled; imports do not overwrite the clipboard.
History's seconds field currently measures processing time for imports.

There is no application duration or compressed-file-size cap. This first import
path processes the full decoded file, so RAM and free disk remain practical
limits. Private temporary files under the data directory consume about 115 MB
per hour for the converted WAV and are deleted on completion, error, or cancel
(a process crash can leave them behind). Float audio alone needs roughly
230 MB RAM per hour, in addition to model and inference buffers. Standard WAV
container size limits also apply. Multi-hour jobs have not been endurance-tested.
Chunked/resumable file imports are not implemented yet. Dictation remains a
short-form microphone workflow with clipboard copy and history saved on stop.
Recorded meetings use a separate two-track capture path described below.

## Optional Nemotron speaker labels

In Model Garage, prepare **Nemotron 3**, then enable **Identify speakers** in
the file import panel. Imports still use the selected Whisper model and
English/German/auto setting. Whisper supplies word timestamps; Nemotron supplies
speaker activity; AFK assigns each word by temporal overlap and groups adjacent
words into timestamped turns. Labels and structured turns are retained in
History and included when copying text. Names are generic and local to each
recording; no voice enrollment or identity recognition is performed.

For file imports, word timings and speaker assignments are estimates. Near-tied overlapping speakers are labeled
"Overlapping speakers", and words outside detected speech are "Unassigned".
The recording is processed sequentially by Whisper and Nemotron to avoid
keeping both models in GPU memory simultaneously. Nemotron uses its stateful
v3-offline streaming preset; it does not use the short-recording-only
full-attention mode. Existing full-file conversion/Whisper memory limits remain.

The optional Q8 model is 107,012,128 bytes, checked against SHA-256
`08456d9e22cd9a323c0364d98375f3746d6e68507ebb705cd46438c534c7a3a1`.
It comes from NVIDIA's
[Nemotron-3-Diarization](https://huggingface.co/nvidia/Nemotron-3-Diarization)
revision `f667ed73aee57d40cc39428eb768b4fd87a0a29e`.
The native [NeMo-Speech.cpp](https://github.com/NVIDIA/NeMo-Speech.cpp) runtime
is pinned to `2bdfb03b38d5e3b0fa55aeb91a079a9f57385373`.
No Python/PyTorch runtime is needed.

First preparation needs network access, Git, Bash, CMake, Ninja, a C++ compiler,
and CUDA at /opt/cuda. Like the existing Whisper setup, the CUDA build currently
targets SM89. The pinned runtime builds its SentencePiece dependency locally.
Build/cache size exceeds the model download size. Preparation can be cancelled;
diagnostics are in `nemotron-prepare.log` under the data directory. Model removal
keeps the compiled runtime for reuse. After preparation, local inference needs
no network access.


## Recorded meetings

Studio has separate Dictation and Meeting recorder cards. Select **Your microphone**
and **Meeting audio**, then **Record meeting**. **Stop & save** saves both tracks;
it does not start inference. Recording requires no Whisper or Nemotron model and
runs no transcription or diarization engine. Both selectors offer **System default**:
the current default microphone and output monitor are resolved at recording start.
Existing explicit device choices are preserved; an ongoing recording keeps its
chosen devices even if the system defaults change. Closing either window continues
recording in the tray. The global shortcut starts the mode selected on its card
and stops active capture.

The **Recorded meetings** list survives application restarts. Choose
**Process recording** whenever ready, using the current Whisper model and language
from Studio's recognition controls. Prepare Whisper and Nemotron in Model Garage
before processing. Cancel processing retains the audio for another attempt.
Processing again replaces the same meeting's History entry, when History is enabled.
A previously completed transcript remains on disk if reprocessing is cancelled.

Meeting audio can be an output monitor (everything played on it) or one active app
stream. Start audio in Teams/browser before refreshing the source list. Refresh
and reselect if the call creates a new stream. AFK doesn't change default devices.
A disconnected source stops recording with an error and retains captured audio.
Use headphones: acoustic echo cancellation isn't implemented, so loudspeaker
bleed into your microphone can appear twice.

During processing, your microphone is labeled **You** and a persistent Nemotron
session assigns generic numbers to remote speakers (up to eight). Labels and word
timings are estimates; overlap can remain ambiguous. Whisper reads bounded chunks
of 28 seconds with one second of previous context. Nemotron retains speaker state
across chunks. Both models may occupy GPU memory during processing; neither runs
while recording. Multi-hour endurance has not been verified.

Private folders under `.afk/meetings/<timestamp>/` contain `microphone.wav`,
`meeting-audio.wav`, and `recording.json`. Audio headers are checkpointed every
half second. Processing adds `engine.log`, a partial `transcript.pending.json`,
and, on success, `transcript.json` plus a completion marker. Raw audio is retained
even with History disabled; deleting History does not delete recording folders.
Processed text can be copied from Studio or History without automatically replacing
the clipboard. Two PCM16 mono tracks consume about 230 MB/hour. Free disk and the
per-track WAV limit (about 37 hours) apply; there is no duration timer.

Capture needs `pactl` and `parec` (Arch: `libpulse`) and the PulseAudio server or
PipeWire's PulseAudio compatibility server. A native speaker helper is compiled
on first processing, using the compiler already needed for model preparation.
`scripts/test-meeting-audio.py` verifies model-free dual recording, restart recovery,
explicit processing, cancellation and persisted transcripts using isolated virtual
audio devices and prerecorded fixtures, never the physical microphone/speakers.

## Installable Arch package

Run `make package` to build `dist/afk-0.1.0-1-x86_64.pkg.tar.zst`, including
precompiled Whisper and Nemotron engines. Install it with `sudo pacman -U`.
No AUR submission is involved. Build dependencies, installed data paths, NVIDIA
requirements and the GitHub Actions draft-release workflow are documented in
[packaging/README.md](packaging/README.md).
