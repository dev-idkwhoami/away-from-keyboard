# Arch package and GitHub builds

`make package` creates `dist/afk-<VERSION>-1-x86_64.pkg.tar.zst` and its SHA-256
checksum. Nothing is uploaded to the AUR. Install with:

```sh
sudo pacman -U dist/afk-0.1.0-1-x86_64.pkg.tar.zst
```

The package contains AFK's desktop and console executables, precompiled patched
Whisper, Nemotron and AFK's speaker helper, a desktop launcher, an icon, and
third-party notices. It never includes your `.afk` data, recordings or model
weights. Models are downloaded and checksum-verified from Model Garage.
Installed data lives in `$XDG_DATA_HOME/afk` (normally `~/.local/share/afk`).
Development builds keep their existing `.afk` location. `--data` overrides both.

## Build requirements

Build as an ordinary user on up-to-date Arch/CachyOS, with Bun 1.3.13 and:

```sh
sudo pacman -S --needed base-devel go git cmake ninja pkgconf gtk3 webkit2gtk-4.1 \
  sdl2 ffmpeg libpulse libx11 cuda nvidia-utils patchelf python ca-certificates
make package
```

The first build fetches pinned Whisper/NeMo/SentencePiece sources and compiles
CUDA code. It is substantially slower than `make build`; subsequent builds reuse
`work/package-runtime`. `PACKAGE_JOBS=6 make package` increases parallelism if
RAM permits. The default is 2 for smaller CI runners. Native runtimes are built
without `-march=native` or NCCL. The current CPU backend requires AVX2/FMA/F16C/BMI2.
CUDA architecture 75 supplies Turing machine code and forward-compatible PTX for
newer GPUs, which may take time to JIT at first use. `PACKAGE_CUDA_ARCH=89` can
make a machine-specific development package; do not distribute it as the default.

The first package targets NVIDIA GPUs (Turing or newer), a compatible driver,
and the CUDA major version used to build it. It uses Arch's `cuda` package;
this is a large toolkit dependency, even though users never compile anything.
A smaller separately bundled CUDA runtime can be added later. No driver kernel
module is shipped by AFK. KDE Plasma is currently required for the app-managed
global shortcut; XWayland is used for reliable bubble positioning on Wayland.
The system needs PulseAudio or PipeWire's PulseAudio compatibility service.

`make package` uses a clean `bin/package/` for executables with installed paths,
preserving your development executables. `make clean` clears `bin/`, not
archives in `dist/` or native caches. Delete `work/package-runtime` to force native rebuilds.

## GitHub Actions

`.github/workflows/package.yml` runs on pushes to `master`, manually from the Actions page, or when a
`vN.N.N` tag matching `VERSION` is pushed. Manual runs upload a downloadable
artifact; tag runs also create an **unpublished draft Release** with the package
and checksum attached. There is no AUR workflow and no automatic publication.
The workflow must be present on the default branch to appear under Run workflow.
Repository/organization settings must allow GitHub Actions; the tag-only release
job needs `contents: write` for the normal `GITHUB_TOKEN`, with no PAT required.

The Ubuntu 24.04 runner runs a disposable `archlinux:base` container, performs a
full Arch update and builds as a non-root user for makepkg. Go and native tools
come from Arch; Bun is pinned. Action references are immutable commits. No GPU
is required to compile; CI checks package layout, ELF loading, engine help/version
startup, Go tests and frontend types. Actual GPU inference is a separate hardware
check. No audio recording or GUI session is required by CI.

Unused hosted-runner SDKs are removed to make room for CUDA; package download
cache is cleared after installation. The job allows 180 minutes with two compiler
jobs. Native sources and compiled engines are cached after a successful package
build and test run. Exact cache keys include the native source/build scripts and
the installed compiler, CMake, CUDA, glibc and SDL versions. There is no partial-key
fallback: a toolchain or engine change gets a clean build. Frontend changes reuse
the engines. Cache misses or eviction simply rebuild them; models and user data
are never cached. The container always mounts the source at `/src`, preserving
CMake's build paths between runs. The Arch image and repositories
are rolling, so this is not a bit-for-bit reproducible build; the installed package
includes `build-packages.txt` recording exact toolchain/dependency versions.

AFK currently has no project-level redistribution license. Package metadata says
all rights reserved rather than inventing a license; choose one before inviting
public reuse. Dependency notices are collected in `/usr/share/licenses/afk`.

References used for the workflow and package layout:
- https://docs.github.com/en/actions/reference/runners/github-hosted-runners
- https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions
- https://github.com/actions/upload-artifact
- https://cli.github.com/manual/gh_release_create
- https://man.archlinux.org/man/PKGBUILD.5.en
- https://man.archlinux.org/man/makepkg.8.en

Native caching follows [actions/cache restore/save](https://github.com/actions/cache/tree/main#using-a-combination-of-restore-and-save-actions).
