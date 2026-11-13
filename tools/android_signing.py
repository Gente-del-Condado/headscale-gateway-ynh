#!/usr/bin/env python3
"""Firma estable Android: credenciales locales privadas, nunca argumentos de proceso."""
import argparse
import base64
import hashlib
import json
from pathlib import Path
import secrets
import subprocess
import tempfile


def private_file(path):
    path = Path(path)
    if not path.is_file() or path.is_symlink() or path.stat().st_mode & 0o077:
        raise ValueError('El archivo de firma debe ser regular, privado y sin enlaces: ' + str(path))
    return path


def load_environment(config):
    config = private_file(config).resolve()
    profile = json.loads(config.read_text())
    if profile.get('format') != 1:
        raise ValueError('Formato de firma no compatible')
    for name in ('keystore', 'password_file'):
        if Path(profile[name]).name != profile[name]:
            raise ValueError('El perfil de firma solo admite nombres relativos a su carpeta')
    keystore = private_file(config.parent / profile['keystore'])
    password = private_file(config.parent / profile['password_file']).read_text().strip()
    if not password:
        raise ValueError('La contraseña de firma está vacía')
    return {'HGW_ANDROID_KEYSTORE': str(keystore),
            'HGW_ANDROID_STORE_PASSWORD': password,
            'HGW_ANDROID_KEY_ALIAS': profile['alias'],
            'HGW_ANDROID_KEY_PASSWORD': password}


def initialize(directory, keytool):
    directory = Path(directory).resolve()
    if directory.exists():
        raise ValueError('No se sobrescribe una identidad de firma existente: ' + str(directory))
    directory.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    with tempfile.TemporaryDirectory(prefix='hgw-signing-', dir=directory.parent) as temporary:
        stage = Path(temporary)
        password_file = stage / 'release.password'
        password_file.touch(mode=0o600)
        password_file.write_text(secrets.token_urlsafe(48) + '\n')
        store = stage / 'release.p12'
        subprocess.run([keytool, '-genkeypair', '-alias', 'headscale-gateway',
                        '-keyalg', 'RSA', '-keysize', '4096', '-validity', '10000',
                        '-dname', 'CN=Headscale Gateway', '-storetype', 'PKCS12',
                        '-keystore', str(store), '-storepass:file', str(password_file),
                        '-keypass:file', str(password_file)], check=True)
        store.chmod(0o600)
        certificate = stage / 'release-certificate.pem'
        subprocess.run([keytool, '-exportcert', '-rfc', '-alias', 'headscale-gateway',
                        '-keystore', str(store), '-storepass:file', str(password_file),
                        '-file', str(certificate)], check=True)
        certificate.chmod(0o600)
        profile = stage / 'signing.json'
        profile.touch(mode=0o600)
        profile.write_text(json.dumps({'format': 1, 'keystore': store.name,
                                      'password_file': password_file.name,
                                      'alias': 'headscale-gateway'}, indent=2) + '\n')
        # El cambio de nombre conserva permisos y no deja una identidad a medias.
        stage.rename(directory)
    load_environment(directory / 'signing.json')
    pem = certificate.name
    data = (directory / pem).read_text()
    der = base64.b64decode(''.join(line for line in data.splitlines() if not line.startswith('---')))
    print('Perfil privado de firma: ' + str(directory / 'signing.json'))
    print('Certificado público SHA256: ' + hashlib.sha256(der).hexdigest())
    print('Conserva una copia segura de toda esta carpeta para las futuras actualizaciones.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('init', 'check'))
    parser.add_argument('path', type=Path)
    parser.add_argument('--keytool', default='keytool')
    args = parser.parse_args()
    try:
        if args.action == 'init':
            initialize(args.path, args.keytool)
        else:
            load_environment(args.path)
            print('OK: perfil de firma privado y válido')
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as error:
        parser.exit(1, str(error) + '\n')


if __name__ == '__main__':
    main()
