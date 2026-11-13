#!/usr/bin/env python3
"""Build pinned Go sources without formatting or changing dependency versions."""
import argparse, base64, hashlib, json, os, subprocess
from pathlib import Path
ROOT = Path(__file__).resolve().parent.parent
TARGETS = {
    'linux-amd64': ('linux', 'amd64', 'bin/linux'),
    'linux-arm64': ('linux', 'arm64', 'bin/linux-arm64'),
    'windows-amd64': ('windows', 'amd64', 'bin'),
    'windows-arm64': ('windows', 'arm64', 'bin/windows-arm64'),
    'darwin-amd64': ('darwin', 'amd64', 'bin/macos-amd64'),
    'darwin-arm64': ('darwin', 'arm64', 'bin/macos-arm64'),
}

def source_fingerprint():
    # Vincula los ejecutables a los fuentes efectivos, no a un nombre de carpeta.
    from prepare_git_install import input_hashes
    inputs = {name: value for name, value in input_hashes(ROOT).items()
              if name.startswith('src/') or name == 'VERSION'}
    return hashlib.sha256(json.dumps(inputs, sort_keys=True).encode()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--targets', nargs='+', choices=TARGETS, default=list(TARGETS))
    parser.add_argument('--commercial-public-key', type=Path, help='Enable commercial build using this issuer PUBLIC key file')
    args = parser.parse_args()
    flags = []
    if args.commercial_public_key:
        public = args.commercial_public_key.read_text().strip()
        try:
            decoded = base64.b64decode(public + '=' * (-len(public) % 4), altchars=b'-_', validate=True)
        except ValueError:
            parser.error('Invalid base64url public key')
        if len(decoded) != 32:
            parser.error('Expected a 32-byte PUBLIC Ed25519 key, never a private key')
        flags = ['-tags=commercial', '-ldflags=-X github.com/gentedelcondado/headscale-gateway/internal/licensing.PublicKey=' + public]
    subprocess.run(['go', 'mod', 'verify'], cwd=ROOT / "src", check=True)
    records = []
    version = (ROOT / 'VERSION').read_text().strip()
    fingerprint = source_fingerprint()
    for target in args.targets:
        system, arch, directory = TARGETS[target]
        output = ROOT / directory
        output.mkdir(parents=True, exist_ok=True)
        env = dict(os.environ, CGO_ENABLED='0', GOOS=system, GOARCH=arch)
        programs = [('headscale-gateway', '.'), ('hgw-ca-helper', './tools/ca-helper')]
        if arch == 'amd64' and system in ('linux', 'windows'):
            programs.append(('gui-lab', './tools/gui-lab'))
        for name, package in programs:
            path = output / (name + ('.exe' if system == 'windows' else ''))
            staged = path.with_name(path.name + '.next')
            print(f'Building {target}: {name}', flush=True)
            subprocess.run(['go', 'build', '-mod=readonly', '-buildvcs=false', '-trimpath',
                            *flags, '-o', str(staged), package], cwd=ROOT / "src", env=env, check=True)
            staged.replace(path)
            with path.open('rb') as source:
                digest = hashlib.file_digest(source, 'sha256').hexdigest()
            records.append({'target': target, 'path': str(path.relative_to(ROOT)), 'sha256': digest, 'commercial': bool(flags), 'version': version, 'source_fingerprint': fingerprint})
    if source_fingerprint() != fingerprint:
        raise ValueError('Los fuentes cambiaron durante la compilación; repite antes de distribuir')
    (ROOT / 'bin/build-manifest.json').write_text(json.dumps(records, indent=2) + '\n')

if __name__ == '__main__':
    main()
