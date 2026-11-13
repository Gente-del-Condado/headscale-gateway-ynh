#!/usr/bin/env python3
"""Collect local dependency notices and a conservative, reviewable inventory.
Requires go mod download and npm ci; identifiers are heuristic, not legal advice.
"""
import hashlib, json, os, re, shutil, subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
OUT=ROOT/'src/third_party'

def objects(text):
 dec=json.JSONDecoder();i=0
 while i<len(text):
  while i<len(text) and text[i].isspace():i+=1
  if i==len(text):return
  value,i=dec.raw_decode(text,i);yield value

def classify(text):
 labels=[]
 for label,pattern in [
  ('AGPL',r'GNU AFFERO GENERAL PUBLIC LICENSE'),('LGPL',r'GNU (?:LESSER|LIBRARY) GENERAL PUBLIC LICENSE'),
  ('GPL',r'GNU GENERAL PUBLIC LICENSE'),('MPL-2.0',r'Mozilla Public License(?: Version)? 2\.0'),
  ('Apache-2.0',r'Apache License[\s,]+Version 2\.0|Apache License, Version 2.0'),
  ('MIT',r'Permission is hereby granted, free of charge'),('BSD-3-Clause',r'Neither the name|Neither my name'),
  ('BSD-2-Clause',r'Redistributions in binary form must reproduce'),('ISC',r'Permission to use, copy, modify, and(?:/or)? distribute'),
  ('Public-domain',r'public domain|Unlicense'),('Zlib',r'altered source versions must be plainly marked')]:
  if re.search(pattern,text,re.I):labels.append(label)
 return labels or ['REVIEW']

def main():
 OUT.mkdir(exist_ok=True)
 used=set()
 for system in ('linux','windows','darwin'):
  for arch in ('amd64','arm64'):
   raw=subprocess.check_output(['go','list','-deps','-f','{{with .Module}}{{.Path}}{{end}}','.','./tools/ca-helper'],cwd=ROOT / "src",env=dict(os.environ,CGO_ENABLED='0',GOOS=system,GOARCH=arch),text=True)
   used.update(raw.split())
 modules=list(objects(subprocess.check_output(['go','list','-m','-json','all'],cwd=ROOT / "src",text=True)))
 rows=[]
 for module in modules:
  if module.get('Main'):continue
  name=module['Path'];version=module.get('Version','');directory=module.get('Dir')
  if not directory:
   value=json.loads(subprocess.check_output(['go','mod','download','-json',name+'@'+version],cwd=ROOT / "src",text=True));directory=value.get('Dir')
  row={'ecosystem':'Go','name':name,'version':version,'linked_server_or_helper':name in used,'notices':[],'license_hints':[]}
  if directory:
   base=Path(directory)
   # Copy root notices and nested notices belonging to shipped module source.
   for file in sorted(base.rglob('*')):
    if not file.is_file() or not re.match(r'^(LICENSE|LICENCE|COPYING|NOTICE|PATENTS)(?:[.\-_].*)?$',file.name,re.I):continue
    if file.stat().st_size>4*1024*1024:continue
    relative=file.relative_to(base)
    target=OUT/'go'/(name+'@'+version)/relative
    target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(file,target)
    row['notices'].append(str(target.relative_to(ROOT)))
    if len(relative.parts)==1:row['license_hints']+=classify(file.read_text(errors='replace'))
  row['license_hints']=sorted(set(row['license_hints'])) or ['REVIEW']
  rows.append(row)
 for file in sorted((ROOT/'src/desktop/node_modules').rglob('package.json')):
  if file.parent.name=='dist':continue
  try:p=json.loads(file.read_text())
  except (ValueError,OSError):continue
  if not p.get('name') or not p.get('version'):continue
  row={'ecosystem':'npm','name':p['name'],'version':p['version'],'scope':'desktop build tooling','declared_license':p.get('license','REVIEW'),'notices':[]}
  for notice in file.parent.iterdir():
   if notice.is_file() and re.match(r'^(LICENSE|LICENCE|COPYING|NOTICE)(?:[.\-_].*)?$',notice.name,re.I):
    target=OUT/'npm'/(p['name']+'@'+p['version'])/notice.name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(notice,target);row['notices'].append(str(target.relative_to(ROOT)))
  rows.append(row)
 for name in ('LICENSE','LICENSES.chromium.html'):
  source=ROOT/'src/desktop/node_modules/electron/dist'/name
  target=OUT/'electron'/name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,target)
 goroot=Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip())
 for name in ('LICENSE','PATENTS'):
  if (goroot/name).exists():
   target=OUT/'go-toolchain'/name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(goroot/name,target)
 (OUT/'inventory.json').write_text(json.dumps(rows,ensure_ascii=False,indent=2)+'\n')
 linked=[r for r in rows if r.get('linked_server_or_helper')]
 review=[r for r in linked if any(x in r['license_hints'] for x in ('REVIEW','GPL','LGPL','AGPL','MPL-2.0'))]
 print(json.dumps({'go_modules':sum(r['ecosystem']=='Go' for r in rows),'linked_modules':len(linked),'npm_records':sum(r['ecosystem']=='npm' for r in rows),'linked_review':[{k:r[k] for k in ('name','version','license_hints')} for r in review]},indent=2))
if __name__=='__main__':main()
