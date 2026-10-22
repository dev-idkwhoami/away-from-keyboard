# Third-party components

AFK uses Wails v3 (MIT), Svelte 5 (MIT), Bits UI (MIT), Tailwind CSS (MIT),
Lucide (ISC), and whisper.cpp (MIT). Their source repositories and dependency
packages carry their respective license notices.

The UI components under `frontend/src/lib/components/ui` are adapted from
[shadcn-svelte](https://github.com/huntabyte/shadcn-svelte) (MIT), using the
component copies already used in the owner's automatic-time-stamps project.
AFK's rough-stroke SVG marks and icons in `Icon.svelte` and `public/afk.svg`
are drawn for this project. The generated neon logo in `frontend/public/afk-neon.png` was created with the built-in image generation tool; its prompt is saved alongside it.

Model downloads come from the pinned whisper.cpp model repository documented
in README.md. AFK does not redistribute model weights inside its executable.


Optional speaker diarization uses NVIDIA
[NeMo-Speech.cpp](https://github.com/NVIDIA/NeMo-Speech.cpp) (Apache-2.0),
its pinned ggml/llama.cpp and SentencePiece dependencies, and
[Nemotron 3 Diarization](https://huggingface.co/nvidia/Nemotron-3-Diarization)
(OpenMDW 1.1). Versions and the model checksum are recorded in README.md.
Their upstream LICENSE, NOTICE, and THIRD_PARTY_NOTICES.md files accompany the
source checkout. Model weights are downloaded separately, not embedded in AFK.
Native integration validation uses the upstream 60-second AMI Meeting Corpus
fixture, credited in NeMo-Speech.cpp's test_files/diar/README.md (CC BY 4.0).

The vendored C API headers in `internal/engine/native/nemo_speech/` come from
the same pinned NeMo-Speech.cpp revision (Apache-2.0); their SPDX notices are
retained. They are used to compile AFK’s small streaming diarization helper.
