#!/usr/bin/env python3
"""Verifica sumas de entrega y excluye secretos y estado de las fuentes propias."""
import hashlib, sys, zipfile
from pathlib import Path
root=Path(__file__).resolve().parent.parent
out=Path(sys.argv[1]).resolve() if len(sys.argv)>1 else root/'releases'
for line in (out/'SHA256SUMS').read_text().splitlines():
 digest,name=line.split('  ',1)
 path=out/name
 if path.parent!=out:raise SystemExit('Invalid manifest path')
 with path.open('rb') as stream:actual=hashlib.file_digest(stream,'sha256').hexdigest()
 if actual!=digest:raise SystemExit('Checksum mismatch: '+name)
 print('OK',name)
with zipfile.ZipFile(out/'headscale-gateway-fuentes.zip') as archive:
 names=archive.namelist()
 for name in names:
  parts=Path(name).parts
  if name.startswith('/') or '..' in parts:raise SystemExit('Unsafe archive entry')
  if any(part in ('node_modules','bin','release','data-lab','data-lab-linux','.git','__pycache__') for part in parts):raise SystemExit('Unexpected generated/private directory: '+name)
  if name.endswith(('.sqlite','.sqlite-wal','.sqlite-shm','.key','.pfx','.p12','.license','.log','.hgw','.hgwcfg')):raise SystemExit('Unexpected private state: '+name)
 required=['AGENTS.md','mkdocs.yml','requirements-docs.txt','tools/manual_theme/main.html','tools/check_manual.py','README.md','LICENSE','NOTICE','src/go.mod','src/go.sum','src/desktop/package-lock.json','src/internal/licensing/license.go','src/third_party/inventory.json','docs/VERIFICACION_FINAL.md']
 for name in required:
  if 'headscale-gateway/'+name not in names:raise SystemExit('Missing source: '+name)
 if archive.testzip() is not None:raise SystemExit('Corrupt source ZIP')
 print('OK source layout:',len(names),'files')
