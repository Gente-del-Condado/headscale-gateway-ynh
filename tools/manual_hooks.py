"""Integra documentación existente en MkDocs y genera una búsqueda local.

Los documentos de plataforma se editan en packaging/ y Android. Las páginas
virtuales y el índice se generan durante la compilación, sin escribir en docs/.
"""
import json
import posixpath
import re
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import quote, unquote, urlsplit, urlunsplit

from mkdocs.structure.files import File

ROOT = Path(__file__).resolve().parents[1]
SOURCES = {
    'ESCRITORIO_LICENCIAS.md': 'packaging/desktop/README.md',
    'HOME_ASSISTANT.md': 'packaging/homeassistant/DOCS.md',
    'INTEGRACION_HA.md': 'packaging/hacs/README.md',
    'YUNOHOST.md': 'packaging/yunohost/doc/ADMIN.md',
    'CATALOGO_YUNOHOST.md': 'packaging/yunohost/doc/CATALOGO.md',
    'ANDROID_CANALES.md': 'packaging/android/README.md',
    'ANDROID_FDROID.md': 'packaging/android/fdroid/README.md',
    'ANDROID_PLAY.md': 'packaging/android/play/PUBLICACION.md',
    'ANDROID_USO.md': 'android/README.md',
    'DESARROLLO.md': 'CONTRIBUTING.md',
    'CONTINUIDAD.md': 'AGENTS.md',
}
# Se respetan los nombres TXT usados por paquetes y enlaces existentes.
for filename in ('API_CLI_MANUAL', 'PORTAL_Y_FEDERACION', 'HELPER_CA',
                 'IDIOMAS_Y_REGISTROS', 'FEDERACION_WINDOWS', 'LEEME_BINARIOS'):
    page = 'API_CLI' if filename == 'API_CLI_MANUAL' else filename
    SOURCES[page + '.md'] = 'docs/' + filename + '.txt'

SEARCH = []
LINK = re.compile(r'(!?\[[^\]\n]*\]\()([^\s)]+)([^)\n]*\))')


class PlainText(HTMLParser):
    """Extrae texto legible para buscar sin insertar HTML en el navegador."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.parts = []

    def handle_data(self, data):
        self.parts.append(data)


def on_config(config):
    SEARCH.clear()
    config.extra['version'] = (ROOT / 'VERSION').read_text().strip()
    return config


def on_files(files, config):
    # El texto legal se descarga también desde ambos manuales públicos.
    for name in ('LICENSE', 'NOTICE'):
        files.append(File.generated(config, name + '.txt', abs_src_path=str(ROOT / name)))
    for page, source in SOURCES.items():
        content = (ROOT / source).read_text(encoding='utf-8')
        if source.endswith('.txt'):
            first, _, body = content.partition('\n')
            content = '# ' + first + '\n' + body
        files.append(File.generated(config, page, content=content))
    # Los ejemplos son descargables desde el manual, también en una subruta.
    for source in sorted((ROOT / 'examples').glob('*')):
        if source.is_file():
            files.append(File.generated(config, 'examples/' + source.name,
                                        abs_src_path=str(source)))
    return files


def on_page_markdown(markdown, page, config, files):
    source = ROOT / SOURCES.get(page.file.src_uri, 'docs/' + page.file.src_uri)
    destinations = {str((ROOT / path).resolve()): name for name, path in SOURCES.items()}

    def resolve_link(match):
        parts = urlsplit(match[2])
        if parts.scheme or parts.netloc or not parts.path or parts.path.startswith('/'):
            return match[0]
        target = (source.parent / unquote(parts.path)).resolve()
        destination = destinations.get(str(target))
        if destination is None and target.is_relative_to(ROOT / 'docs'):
            destination = target.relative_to(ROOT / 'docs').as_posix()
        if destination is None and target.is_relative_to(ROOT / 'examples'):
            destination = target.relative_to(ROOT).as_posix()
        if destination is not None:
            relative = posixpath.relpath(destination, posixpath.dirname(page.file.src_uri) or '.')
            url = urlunsplit(('', '', quote(relative), parts.query, parts.fragment))
        elif target.is_relative_to(ROOT) and target.exists():
            browser = config.extra.get('source_browser', config.repo_url + '/src/branch/main/')
            url = browser + quote(target.relative_to(ROOT).as_posix())
            if parts.fragment:
                url += '#' + parts.fragment
        else:
            # MkDocs debe informar de enlaces rotos, en vez de ocultarlos.
            return match[0]
        return match[1] + url + match[3]

    return LINK.sub(resolve_link, markdown)


def on_page_content(html, page, config, files):
    # Los ejemplos desplazables se pueden recorrer con el teclado.
    html = html.replace('<pre>', '<pre tabindex="0">')
    html = html.replace('<th>', '<th scope="col">')
    text = PlainText()
    text.feed(html)
    SEARCH.append({'title': page.title, 'url': page.url,
                   'text': ' '.join(' '.join(text.parts).split())})
    return html


def on_post_build(config):
    # Un script local permite buscar también al abrir index.html con file://.
    data = json.dumps(SEARCH, ensure_ascii=False).replace('</', '<\\/')
    output = Path(config.site_dir) / 'assets/search-data.js'
    output.write_text('window.HGW_SEARCH = ' + data + ';\n', encoding='utf-8')
