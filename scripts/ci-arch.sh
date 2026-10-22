#!/usr/bin/env bash
# Executed as root inside a disposable Arch container, never on the host.
set -euo pipefail
[[ -e /etc/arch-release && $EUID == 0 && -e /usr/local/bin/bun ]] || exit 1
cd /src
case ${1:-} in
  setup)
    pacman -Syu --noconfirm --needed base-devel go git cmake ninja pkgconf \
        gtk3 webkit2gtk-4.1 sdl2 ffmpeg libpulse libx11 cuda nvidia-utils \
        patchelf python ca-certificates
    pacman -Scc --noconfirm
    useradd --create-home --uid 1000 builder
    # Native ABI/build dependencies only: frontend/Go updates should not rebuild CUDA.
    {
      pacman -Q gcc gcc-libs glibc binutils cmake ninja cuda cccl
      pkg-config --modversion sdl2
    } > work/ci-native-toolchain.txt
    ;;
  build)
    # Failure diagnostics must remain readable by the host runner (different UID).
    trap 'find /src/work -type f \( -name "*.log" -o -name CMakeConfigureLog.yaml \) -exec chmod a+r {} + 2>/dev/null || true' EXIT
    # Restored cache files belong to the host runner, not the container builder.
    chown -R builder:builder /src
    runuser -u builder -- env GOCACHE=/src/work/go-cache PACKAGE_JOBS=2 bash -c '
      set -euo pipefail
      make package
      make test
    '
    ;;
  *) echo 'Usage: ci-arch.sh setup|build' >&2; exit 2 ;;
esac
