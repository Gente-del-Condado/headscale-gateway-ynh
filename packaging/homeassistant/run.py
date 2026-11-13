#!/usr/bin/env python3
"""Adapta las opciones de Supervisor al contrato de arranque común del Gateway."""
import json
import os
from pathlib import Path
import sys
from hgw_bootstrap import prepare as prepare_startup
import urllib.request


def print_access_instructions(stream=None):
    print('''
============================================================
HOME ASSISTANT · PANEL DE ADMINISTRACIÓN
============================================================
En Home Assistant, abre:
  Ajustes > Aplicaciones/Complementos > Headscale Gateway
  Pulsa ABRIR INTERFAZ WEB (Open Web UI).

Activa MOSTRAR EN LA BARRA LATERAL (Show in sidebar) para
abrir el panel directamente desde el menú de Home Assistant.

No necesitas un navegador en el servidor: utiliza la aplicación
de Home Assistant o su web desde tu móvil o PC.
Entrarás directamente con tu sesión de administrador de Home Assistant.
No tienes que introducir otra contraseña en el Gateway.
El acceso directo fuera de Home Assistant conserva su autenticación propia.
============================================================
''', file=stream or sys.stdout, flush=True)


def prepare(options, ingress_port, data=Path('/data'), runtime=Path('/run/headscale-gateway')):
    lan_port = int(options.get('lan_port', 8443))
    if not 1 <= lan_port <= 65535 or not 62000 <= ingress_port <= 65500 or lan_port == ingress_port:
        raise ValueError('El puerto LAN o de Ingress no es válido, o ambos puertos coinciden')
    credentials = {}
    if options.get('admin_password'):
        credentials = {'username': options.get('admin_username', 'admin'), 'password': options['admin_password']}
    return prepare_startup(data, runtime, f'0.0.0.0:{ingress_port}',
        node_name=options.get('node_name', 'homeassistant-gateway'),
        lan_listen_addr=f'0.0.0.0:{lan_port}', base_path_header='X-Ingress-Path',
        trusted_proxies=['172.30.32.2'], allow_frame=True,
        proxy_auth={'user_header': 'X-Remote-User-Id', 'groups_header': 'X-Remote-User-Groups',
                    'admin_group': 'system-admin'}, **credentials)


def supervisor_port(info):
    data = info.get('data') if isinstance(info, dict) else None
    if not isinstance(data, dict) or info.get('result') != 'ok' or data.get('ingress') is not True:
        raise RuntimeError('Supervisor no ha confirmado que Ingress esté habilitado para este complemento')
    # /addons/self/info expone ingress_panel (visibilidad en la barra lateral),
    # no panel_admin. El manifiesto y la comprobación de grupo en cada petición
    # exigen permisos de administrador; un campo ausente no equivale a falso.
    if 'panel_admin' in data and data['panel_admin'] is not True:
        raise RuntimeError('El panel debe estar reservado a administradores de Home Assistant')
    port = data.get('ingress_port')
    if isinstance(port, bool) or not isinstance(port, int) or not 62000 <= port <= 65500:
        raise RuntimeError('Supervisor no ha devuelto un puerto dinámico de Ingress válido')
    return port


def main():
    os.umask(0o077)
    with Path('/data/options.json').open() as stream:
        options = json.load(stream)
    request = urllib.request.Request('http://supervisor/addons/self/info', headers={
        'Authorization': 'Bearer ' + os.environ['SUPERVISOR_TOKEN'],
    })
    with urllib.request.urlopen(request, timeout=15) as response:
        info = json.load(response)
    db, deployment = prepare(options, supervisor_port(info))
    env = dict(os.environ, GATEWAY_BOOTSTRAP_FILE=str(deployment))
    # El Gateway ya no necesita acceso a Supervisor tras obtener el puerto Ingress.
    env.pop('SUPERVISOR_TOKEN', None)
    if options.get('log_level'):
        env['GATEWAY_LOG_LEVEL'] = options['log_level']
    print_access_instructions()
    os.execve('/usr/local/bin/headscale-gateway', ['headscale-gateway', '--db', str(db)], env)


if __name__ == '__main__':
    main()
