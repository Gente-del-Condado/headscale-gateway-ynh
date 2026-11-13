#!/usr/bin/env python3
"""Verificaciones estáticas del panel y de los recursos HTTP embebidos.

Solo utiliza la biblioteca estándar. No inicia el Gateway ni lee secretos.
"""
from __future__ import annotations

import re
import sys
from html.parser import HTMLParser
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "src"
ADMIN = ROOT / "internal" / "admin"


class PanelParser(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.ids: list[str] = []
        self.scripts: list[str] = []
        self.stylesheets: list[str] = []

    def handle_starttag(self, tag, attrs):
        attr = dict(attrs)
        if attr.get("id"):
            self.ids.append(attr["id"])
        if tag == "script" and attr.get("src"):
            self.scripts.append(attr["src"])
        if tag == "link" and attr.get("rel") == "stylesheet" and attr.get("href"):
            self.stylesheets.append(attr["href"])


def check() -> list[str]:
    errors: list[str] = []
    parser = PanelParser()
    parser.feed((ADMIN / "panel.html").read_text(encoding="utf-8"))
    for element_id in set(parser.ids):
        if parser.ids.count(element_id) > 1:
            errors.append(f"ID HTML duplicado: {element_id}")
    # Mantener el manifiesto HTTP y los archivos embebidos sincronizados.
    manifest = (ADMIN / "routes_static.go").read_text(encoding="utf-8")
    served = set(re.findall(r'"(/admin/[\w.-]+)":\s*\{', manifest))
    for asset_url in parser.scripts + parser.stylesheets:
        if not asset_url.startswith("/"):
            asset_url = "/admin/" + asset_url
        if asset_url.startswith("/admin/") and asset_url not in served:
            errors.append(f"Recurso HTML no servido por Go: {asset_url}")
    for url in served:
        if not (ADMIN / url.removeprefix("/admin/")).is_file():
            errors.append(f"Recurso HTTP inexistente: {url}")
    if not (ADMIN / "panel-globals.d.ts").is_file():
        errors.append("Faltan los tipos TypeScript del panel")
    if not (ROOT / "locale" / "en.po").is_file():
        errors.append("Falta el catálogo inglés de Poedit")
    return errors


if __name__ == "__main__":
    failures = check()
    if failures:
        for message in failures:
            print("ERROR:", message, file=sys.stderr)
        sys.exit(1)
    print("OK: IDs, recursos web, tipado y catálogos")
