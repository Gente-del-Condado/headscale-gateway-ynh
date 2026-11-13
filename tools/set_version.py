#!/usr/bin/env python3
"""Establece o comprueba la versión común exigida por los manifiestos de plataforma."""
import argparse
import json
from pathlib import Path
import re
import tomllib
from yunohost_metadata import readme as yunohost_readme

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('version', nargs='?')
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    version = args.version or (ROOT / 'VERSION').read_text().strip()
    if not re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+', version):
        parser.error('Use a version such as 1.2.0')
    hacs = ROOT / 'custom_components/headscale_gateway/manifest.json'
    app = ROOT / 'packaging/homeassistant/config.yaml.in'
    ynh = ROOT / 'packaging/yunohost/manifest.toml'
    desktop = ROOT / 'src/desktop/package.json'
    desktop_lock = ROOT / 'src/desktop/package-lock.json'
    if args.check or not args.version:
        assert (ROOT / 'src/VERSION').read_text().strip() == version
        assert json.loads(hacs.read_text())['version'] == version
        assert re.search(r'^version: (.+)$', app.read_text(), re.M)[1] == version
        assert tomllib.loads(ynh.read_text())['version'].split('~')[0] == version
        assert json.loads(desktop.read_text())['version'] == version
        assert json.loads(desktop_lock.read_text())['version'] == version
        assert json.loads(desktop_lock.read_text())['packages']['']['version'] == version
        assert (ROOT / 'packaging/yunohost/README.md').read_text() == yunohost_readme(ROOT)
    else:
        manifest = json.loads(hacs.read_text()); manifest['version'] = version
        hacs.write_text(json.dumps(manifest, indent=2) + '\n')
        app.write_text(re.sub(r'^version: .+$', 'version: ' + version, app.read_text(), flags=re.M))
        ynh.write_text(re.sub(r'^version = ".+"$', 'version = "' + version + '~ynh1"', ynh.read_text(), flags=re.M))
        (ROOT / 'VERSION').write_text(version + '\n')
        (ROOT / 'src/VERSION').write_text(version + '\n')
        package = json.loads(desktop.read_text()); package['version'] = version
        desktop.write_text(json.dumps(package, indent=2) + '\n')
        lock = json.loads(desktop_lock.read_text())
        lock['version'] = lock['packages']['']['version'] = version
        desktop_lock.write_text(json.dumps(lock, indent=2) + '\n')
        (ROOT / 'packaging/yunohost/README.md').write_text(yunohost_readme(ROOT))
    print('Release version: ' + version)


if __name__ == '__main__':
    main()
