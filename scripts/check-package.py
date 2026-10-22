#!/usr/bin/env python3
"""Validate package layout, private-library resolution and installed CLI startup."""
from pathlib import Path
import os
import subprocess
import sys
import tempfile

package = Path(sys.argv[1]).resolve()
with tempfile.TemporaryDirectory(prefix='afk-package-') as temp:
    root = Path(temp)
    env = os.environ | {'XDG_DATA_HOME': str(root / 'userdata'), 'CUDA_VISIBLE_DEVICES': ''}
    env.pop('LD_LIBRARY_PATH', None)
    env.pop('DISPLAY', None)
    env.pop('WAYLAND_DISPLAY', None)
    subprocess.run(['bsdtar', '-xf', str(package), '-C', temp], check=True)
    required = ['usr/bin/afk', 'usr/bin/afk-desktop', 'usr/lib/afk/whisper/whisper-cli',
                'usr/lib/afk/whisper/whisper-stream', 'usr/lib/afk/nemotron/nemo-speech',
                'usr/lib/afk/nemotron/afk-live-diar', 'usr/share/applications/afk.desktop',
                'usr/share/icons/hicolor/scalable/apps/afk.svg', 'usr/share/licenses/afk/THIRD_PARTY.md']
    for name in required:
        assert (root / name).is_file(), f'Missing {name}'
    for file in root.rglob('*'):
        assert '.afk' not in file.parts and file.suffix not in {'.gguf', '.bin', '.wav'}, f'Private data in package: {file}'
        if file.is_symlink():
            assert file.resolve().is_relative_to(root) and file.exists(), f'Broken or external link: {file}'
        elif file.is_file() and file.open('rb').read(4) == b'\x7fELF':
            output = subprocess.check_output(['ldd', str(file)], text=True, env=env)
            assert 'not found' not in output, f'Missing shared library: {file}\n{output}'
            assert '/work/' not in output and '/.afk/' not in output, f'Development runtime leaked: {output}'
    subprocess.run([str(root / 'usr/bin/afk'), 'models'], env=env, check=True, stdout=subprocess.DEVNULL)
    help_output = subprocess.check_output([str(root / 'usr/bin/afk'), 'help'], env=env, text=True)
    assert str(root / 'userdata/afk') in help_output, 'CLI has development data paths'
    help_output = subprocess.check_output([str(root / 'usr/bin/afk-desktop'), '--help'], env=env,
                                          stderr=subprocess.STDOUT, text=True)
    assert str(root / 'userdata/afk') in help_output, 'Desktop has development data paths'
    subprocess.run([str(root / 'usr/lib/afk/whisper/whisper-cli'), '--help'], env=env, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    subprocess.run([str(root / 'usr/lib/afk/nemotron/nemo-speech'), '--version'], env=env, check=True)
print(f'Package layout, ELF dependencies and CLI startup passed: {package.name}')
