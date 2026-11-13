#!/usr/bin/env python3
"""Comprueba las rutas canónicas y los límites entre fuentes y artefactos."""
from pathlib import Path
import json

ROOT = Path(__file__).resolve().parents[1]

def check():
    required = ('src/go.mod', 'src/go.sum', 'src/main.go', 'src/VERSION',
                'src/internal/admin/panel.html', 'src/desktop/package-lock.json',
                'android/src/main/AndroidManifest.xml', 'android/build.gradle',
                'android/third_party/hev-socks5-tunnel/LICENSE',
                'packaging/homeassistant/config.yaml.in', 'packaging/yunohost/manifest.toml',
                'custom_components/headscale_gateway/manifest.json',
                'docs/ESTRUCTURA.md', 'docs/MIGRACION_REPOSITORIO.md',
                'AGENTS.md', 'mkdocs.yml', 'requirements-docs.txt',
                'tools/manual_hooks.py', 'tools/check_manual.py')
    for name in required:
        if not (ROOT / name).is_file():
            raise ValueError('Falta una ruta canónica: ' + name)
    if list(ROOT.glob('*.go')) or (ROOT / 'go.mod').exists():
        raise ValueError('El módulo Go debe residir únicamente en src/')
    if (ROOT / 'android/app').exists():
        raise ValueError('Las fuentes Android deben residir en android/src/')
    version = (ROOT / 'VERSION').read_text().strip()
    if (ROOT / 'src/VERSION').read_text().strip() != version:
        raise ValueError('Sincroniza la versión con tools/set_version.py')
    for row in json.loads((ROOT / 'src/third_party/inventory.json').read_text()):
        for name in row.get('notices', []):
            if not (ROOT / name).is_file():
                raise ValueError('Falta un aviso de licencia: ' + name)
    print('OK: estructura, versión y avisos de terceros')

if __name__ == '__main__':
    check()
