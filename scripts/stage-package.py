#!/usr/bin/env python3
"""Stage an explicit allowlist; never copy user models, recordings or settings."""
import json
import os
from pathlib import Path
import shutil
import subprocess

root = Path(__file__).resolve().parents[1]
stage = root / 'work/package/stage'
shutil.rmtree(stage, ignore_errors=True)
runtimes = json.loads((root / 'work/package/runtimes.json').read_text())

def copy(source, relative):
    destination = stage / relative
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(source, destination)
    destination.chmod(0o755 if os.access(source, os.X_OK) else 0o644)

for name in ('afk', 'afk-desktop'):
    copy(root / 'bin/package' / name, f'usr/bin/{name}')
for engine, executables in [('whisper', ['whisper-cli', 'whisper-stream']),
                            ('nemotron', ['nemo-speech', 'afk-live-diar'])]:
    source = Path(runtimes[engine])
    files = [source / name for name in executables] + list(source.glob('*.so*'))
    for file in files:
        target = Path('usr/lib/afk') / engine / file.name
        if file.is_symlink():
            (stage / target).parent.mkdir(parents=True, exist_ok=True)
            # Flatten only within this engine; no development paths in symlinks.
            (stage / target).symlink_to(file.resolve().name)
        else:
            copy(file, target)
            subprocess.run(['patchelf', '--set-rpath', '$ORIGIN:/opt/cuda/lib64',
                            str(stage / target)], check=True)
    if engine == 'nemotron':
        copy(source / 'afk-ready', 'usr/lib/afk/nemotron/afk-ready')

copy(root / 'packaging/arch/afk.desktop', 'usr/share/applications/afk.desktop')
copy(root / 'frontend/public/afk.svg', 'usr/share/icons/hicolor/scalable/apps/afk.svg')
copy(root / 'packaging/PROJECT-LICENSE', 'usr/share/licenses/afk/PROJECT-LICENSE')
copy(root / 'THIRD_PARTY.md', 'usr/share/licenses/afk/THIRD_PARTY.md')
copy(root / 'README.md', 'usr/share/doc/afk/README.md')
copy(root / 'packaging/README.md', 'usr/share/doc/afk/packaging/README.md')
copy(root / 'THIRD_PARTY.md', 'usr/share/doc/afk/THIRD_PARTY.md')
copy(root / 'frontend/public/afk-neon.png', 'usr/share/doc/afk/frontend/public/afk-neon.png')
for guide in (root / 'docs').glob('*.md'):
    copy(guide, 'usr/share/doc/afk/docs/' + guide.name)
# Preserve license files from native, Go and frontend dependencies. Explicitly
# exclude git internals and build outputs; include nested third-party notices.
license_root = Path('usr/share/licenses/afk/dependencies')

def licenses(source, prefix):
    found = 0
    for directory, dirs, files in os.walk(source):
        dirs[:] = [d for d in dirs if d not in {'.git', '.cache'} and not d.startswith('build')]
        for name in files:
            if name.upper().startswith(('LICENSE', 'LICENCE', 'COPYING', 'NOTICE', 'THIRD_PARTY')):
                file = Path(directory) / name
                copy(file, license_root / prefix / file.relative_to(source))
                found += 1
    return found

whisper_source = Path(runtimes['whisper']).parent.parent
nemo_source = Path(runtimes['nemotron']).parent.parent / 'source'
for source, name in [(whisper_source, 'whisper'), (nemo_source, 'nemotron'),
                     (root / 'frontend/node_modules', 'frontend'),
                     (root / 'frontend/src/lib/components/ui', 'shadcn-svelte')]:
    if not licenses(source, name):
        raise SystemExit(f'Missing third-party notices: {source}')
for module in (root, root / 'desktop'):
    output = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], cwd=module, text=True)
    decoder = json.JSONDecoder()
    while output.strip():
        dep, offset = decoder.raw_decode(output.lstrip())
        output = output.lstrip()[offset:]
        if not dep.get('Main') and dep.get('Path') != 'afk' and dep.get('Dir'):
            licenses(Path(dep['Dir']), 'go/' + dep['Path'] + '@' + dep.get('Version', 'local'))
# Record exact system dependency versions used by this binary build, including
# CUDA major compatibility, instead of silently accepting incompatible upgrades.
versions = subprocess.check_output(['pacman', '-Q', 'go', 'gcc', 'gcc-libs', 'glibc',
    'cmake', 'ninja', 'cuda', 'nvidia-utils', 'gtk3', 'webkit2gtk-4.1',
    'ffmpeg', 'libpulse', 'libx11', 'pkgconf'], text=True)
versions += 'bun ' + subprocess.check_output(['bun', '--version'], text=True)
versions += 'sdl2 ' + subprocess.check_output(['pkg-config', '--modversion', 'sdl2'], text=True)
(stage / 'usr/share/doc/afk/build-packages.txt').write_text(versions)
for directory, _, _ in os.walk(stage):
    Path(directory).chmod(0o755)
print(f'Staged package at {stage}')
