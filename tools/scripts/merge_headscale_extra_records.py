#!/usr/bin/env python3
"""Merge Gateway-created A/AAAA records into a Headscale watched JSON file.

Run on the Headscale/YunoHost host as a privileged operator, after transferring
`gateway-extra-records.json` from the Gateway. Preserve unrelated DNS records.

Example:
 python3 merge_headscale_extra_records.py \
   --source /tmp/gateway-extra-records.json \
   --destination /var/lib/headscale/gateway-extra-records.json \
   --suffix laptop.intra.example.org

The suffix is the EXACT Gateway node's FQDN. Only this node's service names
are replaced. Does not execute shell commands or restart Headscale.
"""
import argparse
import ipaddress
import json
import os
from pathlib import Path
import re
import stat
import tempfile

TAILNET_IPV4 = ipaddress.ip_network('100.64.0.0/10')
TAILNET_IPV6 = ipaddress.ip_network('fd7a:115c:a1e0::/48')
FQDN = re.compile(r'^[a-z0-9-]+(?:\.[a-z0-9-]+)+$', re.ASCII)


def read_records(path):
    if path.exists() and path.stat().st_size > 1024 * 1024:
        raise ValueError(f'Too many DNS records in {path}')
    if not path.exists():
        return []
    with path.open(encoding='utf-8') as stream:
        rows = json.load(stream)
    if not isinstance(rows, list):
        raise ValueError(f'DNS records must be an array in {path}')
    return rows


def merge_records(source, existing, suffix):
    if not FQDN.fullmatch(suffix) or suffix.startswith('-'):
        raise ValueError('Invalid Gateway node FQDN')
    prefix = '.' + suffix
    out = [row for row in existing if not (isinstance(row, dict) and isinstance(row.get('name'), str) and row['name'].lower().endswith(prefix))]
    names = set()
    for row in source:
        if not isinstance(row, dict) or set(row) != {'name', 'type', 'value'}:
            raise ValueError('Each record must have name, type and value')
        name, kind, raw_ip = row['name'], row['type'], row['value']
        if not isinstance(name, str) or not FQDN.fullmatch(name) or not name.endswith(prefix):
            raise ValueError('Record outside Gateway node domain')
        if not isinstance(raw_ip, str):
            raise ValueError('Invalid IP')
        ip = ipaddress.ip_address(raw_ip)
        if (ip.version == 4 and (kind != 'A' or ip not in TAILNET_IPV4) or
            ip.version == 6 and (kind != 'AAAA' or ip not in TAILNET_IPV6)):
            raise ValueError('Record must target a Tailscale IP with matching A/AAAA type')
        identity = (name, kind, raw_ip)
        if identity in names:
            raise ValueError('Duplicate record in Gateway export')
        names.add(identity)
        out.append(row)
    return sorted(out, key=lambda row: (str(row.get('name', '')), str(row.get('type', '')), str(row.get('value', ''))))


def atomic_save(destination, records):
    destination.parent.mkdir(parents=True, exist_ok=True)
    previous = destination.stat() if destination.exists() else None
    mode = stat.S_IMODE(previous.st_mode) if previous is not None else 0o644
    payload = json.dumps(records, ensure_ascii=False, indent=2, sort_keys=True).encode('utf8') + b'\n'
    with tempfile.NamedTemporaryFile(mode='wb', dir=destination.parent, prefix='.headscale-dns-', delete=False) as output:
        tmp = Path(output.name)
        try:
            os.fchmod(output.fileno(), mode)
            # Atomically replacing a file changes its inode and owner. Preserve
            # YunoHost/Headscale file ownership if it already existed.
            if previous is not None:
                os.fchown(output.fileno(), previous.st_uid, previous.st_gid)
            output.write(payload)
            output.flush()
            os.fsync(output.fileno())
        except BaseException:
            tmp.unlink(missing_ok=True)
            raise
    try:
        os.replace(tmp, destination)
    except BaseException:
        tmp.unlink(missing_ok=True)
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--destination', type=Path, required=True)
    parser.add_argument('--suffix', required=True)
    options = parser.parse_args()
    if options.source.resolve() == options.destination.resolve():
        parser.error('--source and --destination must differ')
    merged = merge_records(read_records(options.source), read_records(options.destination), options.suffix.lower())
    atomic_save(options.destination, merged)
    print(f'Saved {len(merged)} DNS records to {options.destination}')


if __name__ == '__main__':
    main()
