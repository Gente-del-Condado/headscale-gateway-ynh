"""Contrato de arranque común para los instaladores; el estado permanece en SQLite."""
import ipaddress
import json
import os
from pathlib import Path
import re


def private_json(path, value):
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    with open(path, 'w', opener=lambda p, flags: os.open(p, flags | getattr(os, 'O_NOFOLLOW', 0), 0o600)) as stream:
        if hasattr(os, 'fchmod'):
            os.fchmod(stream.fileno(), 0o600)
        json.dump(value, stream)


def prepare(data_dir, config_dir, listen_addr, *, node_name='', lan_listen_addr='',
            username=None, password=None, base_path='', base_path_header='',
            trusted_proxies=(), allow_frame=False, credentials_dir=None, proxy_auth=None):
    data_dir, config_dir = Path(data_dir), Path(config_dir)
    if node_name and not re.fullmatch(r'[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', node_name):
        raise ValueError('node_name debe ser un nombre DNS estable en minúsculas')
    if not re.fullmatch(r'(?:/[A-Za-z0-9_-]+)*', base_path):
        raise ValueError('El prefijo de ruta no es válido')
    if base_path_header not in ('', 'X-Ingress-Path') or (base_path_header and base_path):
        raise ValueError('Utiliza un prefijo de ruta fijo o una cabecera de ruta del proxy confiable')
    if base_path_header and not trusted_proxies:
        raise ValueError('Las cabeceras de ruta requieren proxies de confianza configurados')
    for proxy in trusted_proxies:
        ipaddress.ip_address(proxy)
    if proxy_auth is not None:
        header_groups = set(proxy_auth) == {'user_header', 'groups_header', 'admin_group'} and base_path_header
        unix_groups = set(proxy_auth) == {'user_header', 'unix_group', 'shared_secret'}
        if not trusted_proxies or not (header_groups or unix_groups):
            raise ValueError('La autenticación delegada requiere identidad, grupo administrador y un proxy de confianza')
    if username is not None or password is not None:
        if not username or not username.strip() or not password or not 12 <= len(password.encode()) <= 72:
            raise ValueError('Indica un usuario y una contraseña inicial de entre 12 y 72 bytes')
    state = data_dir / 'gateway'
    state.mkdir(mode=0o700, parents=True, exist_ok=True)
    state.chmod(0o700)
    options = {'listen_addr': listen_addr, 'base_path': base_path,
               'base_path_header': base_path_header, 'trusted_proxies': list(trusted_proxies),
               'allow_frame': allow_frame, 'node_name': node_name,
               'lan_listen_addr': lan_listen_addr}
    if proxy_auth is not None:
        options['proxy_auth'] = proxy_auth
    if username is not None:
        initial = Path(credentials_dir or config_dir) / 'initial-admin.json'
        private_json(initial, {'username': username.strip(), 'password': password})
        options['initial_admin_file'] = str(initial)
    private_json(config_dir / 'startup.json', options)
    return state / 'gateway.sqlite', config_dir / 'startup.json'
