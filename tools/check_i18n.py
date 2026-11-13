#!/usr/bin/env python3
"""Check catalog coverage, compiled files and placeholders without writing files."""
from collections import Counter
from pathlib import Path
import re
import runpy

root = Path(__file__).resolve().parents[1]
catalog = runpy.run_path(str(root / 'tools/extract_i18n.py'))
outputs = catalog['catalog_outputs']()
fmt = re.compile(r'%(?:\[[0-9]+\])?[-+# 0]*(?:\d+|\*)?(?:\.(?:\d+|\*))?[a-zA-Z%]')
params = re.compile(r'\{[a-zA-Z0-9_]+\}')


def placeholders(text):
    return Counter(fmt.findall(text) + params.findall(text))


errors = []
for path, expected in outputs.items():
    if not path.is_file() or path.read_text(encoding='utf8') != expected:
        errors.append(f'{path.relative_to(root)} desactualizado. Ejecuta python3 tools/extract_i18n.py')

for directory in ('src/internal/i18n/locales', 'src/desktop/locales'):
    for path in sorted((root / directory).glob('*.json')):
        if path not in outputs:
            errors.append(f'{path.relative_to(root)} carece de catálogo PO')

for path in sorted((root / 'src/locale').glob('*.po')):
    messages = catalog['read_po'](path)
    for key in catalog['refs']:
        translated = messages.get(key)
        if not translated:
            if path.stem == 'en':
                errors.append('en: missing ' + key)
        elif placeholders(key) != placeholders(translated):
            errors.append(path.stem + ': placeholders differ: ' + key)

if errors:
    raise SystemExit('\n'.join(errors))
print('Catalog coverage and placeholders OK:', len(catalog['refs']), 'source messages')
