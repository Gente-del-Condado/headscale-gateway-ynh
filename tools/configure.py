#!/usr/bin/env python3
"""Prepara el archivo de arranque común de Home Assistant y YunoHost."""
import argparse
from getpass import getpass
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'packaging/common'))
from hgw_bootstrap import prepare


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data-dir', type=Path, required=True)
    parser.add_argument('--config-dir', type=Path, required=True)
    parser.add_argument('--listen', default='127.0.0.1:9091')
    parser.add_argument('--node-name', default='')
    parser.add_argument('--base-path', default='')
    parser.add_argument('--trusted-proxy', action='append', default=[])
    parser.add_argument('--allow-frame', action='store_true')
    parser.add_argument('--admin')
    args = parser.parse_args()
    password = getpass('Contraseña inicial del Gateway: ') if args.admin else None
    db, config = prepare(args.data_dir, args.config_dir, args.listen, node_name=args.node_name,
                         base_path=args.base_path, trusted_proxies=args.trusted_proxy,
                         allow_frame=args.allow_frame, username=args.admin, password=password)
    print(f'GATEWAY_BOOTSTRAP_FILE={config}')
    print(f'headscale-gateway --db {db}')


if __name__ == '__main__':
    main()
