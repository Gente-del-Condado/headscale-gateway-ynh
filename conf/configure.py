#!/usr/bin/env python3
import argparse
import json
from hgw_bootstrap import prepare
import os
from pathlib import Path
import re
import secrets
import stat
import sys




def main():
    parser = argparse.ArgumentParser()
    for arg in ('app', 'install-dir', 'data-dir', 'port', 'lan-port', 'path', 'proxy-include'):
        parser.add_argument('--' + arg, required=True)
    parser.add_argument('--admin')
    args = parser.parse_args()
    base = args.path.rstrip('/')
    if not re.fullmatch(r'(?:/[A-Za-z0-9_-]+)*', base):
        raise ValueError('Utiliza una ruta que contenga solo letras, números, guiones y guiones bajos')
    startup = Path(args.install_dir) / 'startup.json'
    secret = None
    if startup.exists() or startup.is_symlink():
        with os.fdopen(os.open(startup, os.O_RDONLY | os.O_NOFOLLOW), 'rb') as stream:
            info = os.fstat(stream.fileno())
            content = stream.read((16 << 10) + 1) if stat.S_ISREG(info.st_mode) else b''
            if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or len(content) > 16 << 10:
                raise ValueError('La configuración de arranque existente debe ser un archivo normal privado de menos de 16 KiB')
            secret = json.loads(content).get('proxy_auth', {}).get('shared_secret')
        if secret is not None and not re.fullmatch(r'[0-9a-f]{64}', secret):
            raise ValueError('El secreto existente del proxy no es válido')
    secret = secret or secrets.token_hex(32)
    prepare(args.data_dir, args.install_dir, '127.0.0.1:' + args.port,
            node_name=args.app.replace('_', '-'), lan_listen_addr='127.0.0.1:' + args.lan_port,
            base_path=base, trusted_proxies=['127.0.0.1', '::1'], allow_frame=True,
            username=args.admin, password=sys.stdin.read() if args.admin else None,
            credentials_dir=Path(args.data_dir),
            proxy_auth={'user_header': 'Ynh-User', 'unix_group': 'admins', 'shared_secret': secret})
    include = Path(args.proxy_include)
    include.parent.mkdir(parents=True, exist_ok=True)
    with open(include, 'w', opener=lambda path, flags: os.open(path, flags | os.O_NOFOLLOW, 0o600)) as stream:
        os.fchmod(stream.fileno(), 0o600)
        stream.write('proxy_set_header X-Gateway-Proxy-Token "' + secret + '";\n')



if __name__ == '__main__':
    main()
