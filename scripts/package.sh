#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD
[[ $(uname -m) == x86_64 ]] || { echo "This package currently targets x86_64 only." >&2; exit 1; }
if (( EUID == 0 )); then echo 'Run make package as an ordinary user (makepkg refuses root).' >&2; exit 1; fi
for tool in go bun cmake ninja git pkg-config c++ makepkg patchelf python3; do
    command -v "$tool" >/dev/null || { echo "Missing build dependency: $tool" >&2; exit 1; }
done
version=$(cat VERSION)
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'VERSION must be N.N.N' >&2; exit 1; }
export GOCACHE=${GOCACHE:-$root/work/go-cache}
export JOBS=${PACKAGE_JOBS:-2}
export CFLAGS='-O2 -pipe' CXXFLAGS='-O2 -pipe' GOAMD64=v1
# Do not inherit host-specific makepkg tuning from CachyOS.
export CGO_ENABLED=1
export CGO_CFLAGS='-O2 -g' CGO_CXXFLAGS='-O2 -g'
mkdir -p work/package dist
make build BIN_DIR=bin/package GO_BUILD_FLAGS=-trimpath GO_LDFLAGS='-s -w -X afk/internal/engine.PackagedRuntimeRoot=/usr/lib/afk'
go run -buildvcs=false ./cmd/afk-runtime --data "$root/work/package-runtime" \
    --jobs "${PACKAGE_JOBS:-2}" --cuda-arch "${PACKAGE_CUDA_ARCH:-75}" > work/package/runtimes.json
python3 scripts/stage-package.py
export AFK_STAGE="$root/work/package/stage" PKGDEST="$root/dist"
cuda_major=$(pacman -Q cuda | awk '{split($2,v,"."); print v[1]}')
sed -e "s/^pkgver=.*/pkgver=$version/" -e "s/'cuda'/'cuda>=$cuda_major' 'cuda<$((cuda_major + 1))'/" packaging/arch/PKGBUILD > work/package/PKGBUILD
# Build only from staged binaries; validate all declared runtime dependencies.
(cd work/package && makepkg --force --noconfirm --config "$root/packaging/arch/makepkg.conf")
(cd dist && sha256sum "afk-$version-1-x86_64.pkg.tar.zst" > "afk-$version-1-x86_64.pkg.tar.zst.sha256")
python3 scripts/check-package.py "dist/afk-$version-1-x86_64.pkg.tar.zst"
