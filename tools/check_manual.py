#!/usr/bin/env python3
"""Comprueba enlaces, anclas y requisitos básicos del manual HTML generado.

No descarga recursos ni modifica archivos. Resuelve rutas desde el documento
de origen, como el navegador, para detectar enlaces que fallarían en Pages.
"""
import argparse
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit


class Document(HTMLParser):
    def __init__(self, path):
        super().__init__(convert_charrefs=True)
        self.path = path
        self.anchors = set()
        self.links = []
        self.errors = []
        self.main_count = 0
        self.h1_count = 0

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if attrs.get('id'):
            if attrs['id'] in self.anchors:
                self.errors.append('ID duplicado: ' + attrs['id'])
            self.anchors.add(attrs['id'])
        if tag == 'main':
            self.main_count += 1
        if tag == 'h1':
            self.h1_count += 1
        for key in ('href', 'src'):
            if attrs.get(key):
                self.links.append(attrs[key])
        if tag == 'img':
            if 'alt' not in attrs:
                self.errors.append('Imagen sin texto alternativo')
            if not attrs.get('width') or not attrs.get('height'):
                self.errors.append('Imagen sin dimensiones: ' + attrs.get('src', '?'))


def check(directory):
    root = directory.resolve()
    documents = {}
    for path in sorted(root.rglob('*.html')):
        document = Document(path)
        document.feed(path.read_text(encoding='utf-8'))
        documents[path] = document
    if root / 'index.html' not in documents:
        raise ValueError('Falta index.html; compila primero con MkDocs')
    errors = []
    links = 0
    for path, document in documents.items():
        prefix = str(path.relative_to(root)) + ': '
        errors.extend(prefix + message for message in document.errors)
        if document.main_count != 1 or document.h1_count != 1:
            errors.append(prefix + 'Debe contener un único main y un único h1')
        for link in document.links:
            parts = urlsplit(link)
            if parts.scheme or parts.netloc:
                continue
            links += 1
            if parts.path.startswith('/'):
                errors.append(prefix + 'Ruta absoluta incompatible con subrutas de Pages: ' + link)
                continue
            target = (path.parent / unquote(parts.path)).resolve() if parts.path else path
            if target.is_dir():
                target /= 'index.html'
            if not target.is_relative_to(root) or not target.is_file():
                errors.append(prefix + 'Recurso local inexistente: ' + link)
            elif parts.fragment and target in documents:
                if unquote(parts.fragment) not in documents[target].anchors:
                    errors.append(prefix + 'Ancla inexistente: ' + link)
    if errors:
        raise ValueError('\n'.join(errors))
    print(f'Manual correcto: {len(documents)} páginas HTML y {links} enlaces locales comprobados')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path, nargs='?', default=Path('builds/manual'))
    args = parser.parse_args()
    try:
        check(args.directory)
    except ValueError as error:
        parser.exit(1, str(error) + '\n')
