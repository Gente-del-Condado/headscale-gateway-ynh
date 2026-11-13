#!/usr/bin/env python3
"""Create a gettext catalog without overwriting an existing language."""
import re,runpy,sys
from pathlib import Path
root=Path(__file__).resolve().parent.parent
if len(sys.argv)!=2 or not re.fullmatch(r'[a-z]{2,3}(?:-[a-z0-9]{2,8})*',sys.argv[1]):
 raise SystemExit('Usage: python3 tools/add_language.py fr (or pt-br)')
lang=sys.argv[1];destination=root/'src/locale'/f'{lang}.po'
if destination.exists():raise SystemExit('Catalog already exists: '+str(destination))
data=runpy.run_path(str(root/'tools/extract_i18n.py'))
destination.write_text(data['render_po'](lang, {}), encoding='utf8')
data['generate']()
print('Edit in Poedit:',destination)
print('Then run: python3 tools/extract_i18n.py and python3 tools/check_i18n.py, rebuild Gateway and package the desktop client.')
