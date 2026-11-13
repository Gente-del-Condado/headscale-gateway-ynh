#!/usr/bin/env python3
"""Extract Spanish gettext msgids, preserve Poedit edits and compile PO to JSON.
Usage: python3 tools/extract_i18n.py
Source language: es; no JavaScript dependencies, no gettext runtime required.
"""
import ast
from collections import defaultdict
from html.parser import HTMLParser
import json
from pathlib import Path
import re
ROOT = Path(__file__).resolve().parent.parent
LOCALE=ROOT/'src/locale'
GO_RE=re.compile(r'"((?:\\.|[^"\\])*)"')
JS_RE=re.compile(r"'(?:\\.|[^'\\])*'|\"(?:\\.|[^\"\\])*\"")

def is_phrase(s):
    s=s.strip()
    return len(s)>=2 and len(s)<500 and ' ' in s and not s.startswith(('http:', 'https:', 'application/', 'GET ', 'POST ', 'PUT ', 'DELETE ', 'function ', '{', '[', '#', '.')) and '\\n' not in s and not ('/' in s and not ' ' in s)

def po_unquote(s):
    return json.loads(s)

def read_po(path):
    if not path.exists():return {}
    messages={}; key=None; val=''; mode=None
    def save():
        if key and val: messages[key]=val
    for line in path.read_text(encoding='utf8').splitlines():
        if line.startswith('msgid '):save();key=po_unquote(line[6:]);val='';mode='id'
        elif line.startswith('msgstr '):val=po_unquote(line[7:]);mode='str'
        elif line.startswith('"'):
            if mode=='id':key+=po_unquote(line)
            elif mode=='str':val+=po_unquote(line)
    save();return messages

def po_quote(s):return json.dumps(s,ensure_ascii=False)

class ExtractHTML(HTMLParser):
 def __init__(self):super().__init__(convert_charrefs=True);self.data=[];self.skip=0
 def handle_starttag(self,tag,attrs):
  if tag in ('code','pre','script','style'):self.skip+=1
  if self.skip:return
  for name,value in attrs:
   if name in ('placeholder','title','aria-label') and value and self.is_label(value):self.data.append(value.strip())
 def handle_endtag(self,tag):
  if tag in ('code','pre','script','style') and self.skip:self.skip-=1
 def is_label(self,value):
  value=value.strip()
  return 2<=len(value)<=700 and not value.startswith(('http://','https://','ldaps://','uid=','dc=')) and value not in ('true','false')
 def handle_data(self,data):
  data=' '.join(data.split())
  if not self.skip and self.is_label(data):self.data.append(data)

refs=defaultdict(set)
for text in (
 'Dirección y puerto TCP del gateway; por ejemplo 0.0.0.0:8443. Admite puertos personalizados; requiere reiniciar. Tailscale conserva HTTPS 443.',
): refs[text].add('admin_schema.go')
for label in ('CA ya aprobada; se conserva su autorización.', 'Tailscale primero (HTTPS 443); después HTTPS LAN en el puerto anunciado. HTTP LAN solo para CA si lo permites. Descargar no implica aprobar.', 'el candidato ya no está disponible; vuelve a descubrir gateways'):
 refs[label].add("admin_federation.go")
for label in ('Consultar','Público; puede requerir activación'):
 refs[label].add('internal/admin/panel-reference.js')
for file in sorted((ROOT/'src/internal'/'admin').glob('*.html'))+sorted((ROOT/'src/desktop').glob('*.html')):
 h=ExtractHTML();h.feed(file.read_text(encoding='utf8'))
 for string in h.data:refs[string].add(str(file.relative_to(ROOT)))
for file in sorted((ROOT/'src/internal'/'admin').glob('*.js'))+sorted((ROOT/'src/desktop').glob('*.js'))+ [p for p in sorted((ROOT/'src/desktop').glob('*.cjs')) if not any(x in p.name for x in ('test','smoke','pack'))]:
 text=file.read_text(encoding='utf8')
 # El extractor estático excluye las plantillas con interpolaciones.
 # Esas frases dinámicas deben usar tr() con identificadores de mensaje constantes.
 for match in JS_RE.finditer(text):
  q,body=match.group(0)[0],match.group(0)[1:-1]
  if '\\' in body:
   try:body=ast.literal_eval(q+body+q)
   except (SyntaxError,ValueError):continue
  if is_phrase(body) and not body.startswith((')',',',':',']')) and not any(x in body for x in ("\n","&&","||","===","=>","document.","el(","return ","await ","const ",";","`","?.",".append("," += "," + "," = ","prefs.","data.online","{cache:","[")):refs[body.strip()].add(str(file.relative_to(ROOT)))
# Explicit user-facing dynamic labels (including single-word messages).
CALL_RE=re.compile(r'''\b(?:hsTr|tr|t)\(\s*('(?:\\.|[^'\\])*'|"(?:\\.|[^"\\])*")''')
for file in sorted((ROOT/'src/internal'/'admin').glob('*.js'))+sorted((ROOT/'src/desktop').glob('*.js'))+ [p for p in sorted((ROOT/'src/desktop').glob('*.cjs')) if not any(x in p.name for x in ('test','smoke','pack'))]:
 for m in CALL_RE.finditer(file.read_text(encoding='utf8')):
  body=ast.literal_eval(m.group(1))
  if len(body)>=2:refs[body].add(str(file.relative_to(ROOT)))
for file in sorted((ROOT / 'src').glob('*.go'))+sorted((ROOT/'src/internal').rglob('*.go')):
 if file.name.endswith('_test.go'):continue
 text=file.read_text(encoding='utf8')
 for m in GO_RE.finditer(text):
  body=m.group(1)
  if '\\' in body:
   try:body=json.loads('"'+body+'"')
   except ValueError:continue
  if is_phrase(body) and not body.startswith((')',',',':',']')) and not any(x in body for x in ("\n","&&","||","===","=>","document.","el(","return ","await ","const ",";","`","?.",".append("," += "," + "," = ","prefs.","data.online","{cache:","[")):refs[body.strip()].add(str(file.relative_to(ROOT)))
refs.update({s:refs.get(s,set()) for s in ('ERROR', 'ADVERTENCIA', 'INFORMACIÓN', 'DEPURACIÓN', 'TRAZA','Plantillas: %d nuevas, %d actualizadas, %d omitidas.', 'Repositorio sincronizado correctamente.', 'No se pudo abrir el archivo de log.', 'Configuración del repositorio guardada.', 'Importación de plantillas finalizada.', 'Sincronización de plantillas fallida.', 'Solicitud HTTP procesada.', 'Cambio administrativo registrado.')})
GO_CALL=re.compile(r'(?:i18n\.T|\btr)\(\s*("(?:\\.|[^"\\])*"|`[^`]*`)')
for file in sorted((ROOT / 'src').glob('*.go'))+sorted((ROOT/'src/internal').rglob('*.go')):
 if file.name.endswith('_test.go'):continue
 for match in GO_CALL.finditer(file.read_text()):
  value=match.group(1);message=value[1:-1] if value.startswith('`') else json.loads(value)
  refs[message].add(str(file.relative_to(ROOT)))
reference_path=ROOT/'src/internal/admin/reference-data.json'
if reference_path.exists():
 data=json.loads(reference_path.read_text())
 for section in ('commands','options','integrations'):
  for item in data.get(section,[]):
   refs[item['description']].add('internal/admin/reference-data.json')
catalog=ROOT/'docs'/'eventos-log.json' 
if catalog.exists():
 for event in json.loads(catalog.read_text()):
  refs[event['message']].add(event['file'])
for key in list(refs):
 if key.startswith(('SELECT ','INSERT ','CREATE TABLE ','PRAGMA ','BEGIN ','VACUUM ')) or key in (': info.manually_paused?', '-ArgumentList @(', 'do shell script','with administrator privileges'):
  del refs[key]
for key in ('Idioma','Solicitudes pendientes: {count}','{count} rutas pendientes de aprobación. Revisa el panel.','conectado','Actualizar','Salir'):
 refs[key].add('desktop/main.cjs')
header='''msgid ""
msgstr ""
"Project-Id-Version: headscale-gateway\\n"
"Content-Type: text/plain; charset=UTF-8\\n"
"Content-Transfer-Encoding: 8bit\\n"
"Language: es\\n"
"X-Generator: Headscale Gateway gettext extractor\\n"
'''
def render_po(language, translations, template=False):
    result = [header.replace('Language: es', 'Language: ' + language)]
    for message in sorted(refs, key=str.casefold):
        for reference in sorted(refs[message]):
            result.append('#: ' + reference)
        fallback = message if language == 'es' and not template else ''
        result.extend(['msgid ' + po_quote(message),
                       'msgstr ' + po_quote(translations.get(message, fallback)), ''])
    return '\n'.join(result)


def catalog_outputs():
    """Genera los archivos esperados desde las fuentes y las traducciones humanas."""
    outputs = {LOCALE / 'headscale-gateway.pot': render_po('es', {}, template=True)}
    languages = {'en', 'es'} | {p.stem for p in LOCALE.glob('*.po')}
    for language in sorted(languages):
        path = LOCALE / (language + '.po')
        translations = read_po(path)
        outputs[path] = render_po(language, translations)
        compiled = {key: value for key, value in translations.items()
                    if key in refs and value and value != key}
        content = json.dumps(compiled, ensure_ascii=False, indent=2, sort_keys=True) + '\n'
        for directory in ('src/internal/i18n/locales', 'src/desktop/locales'):
            outputs[ROOT / directory / (language + '.json')] = content
    return outputs


def generate():
    for path, content in catalog_outputs().items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding='utf8')
    print(f'Catálogos generados para {len(refs)} mensajes')


if __name__ == '__main__':
    generate()
