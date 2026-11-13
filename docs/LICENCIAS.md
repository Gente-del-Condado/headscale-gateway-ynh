# Licencia y avisos de terceros

Revisión técnica: 12 de noviembre de 2026. Se han contrastado los avisos locales,
los metadatos y la distribución de fuentes del motor. Esta revisión no acredita
la titularidad histórica de las aportaciones ni sustituye la revisión de las
obligaciones de cada componente distribuido.

## Código propio

Headscale Gateway se publica bajo **GNU Affero General Public License, versión
3 únicamente** (`AGPL-3.0-only`). El alcance está definido en `NOTICE`: código,
scripts, documentación y recursos originales, salvo materiales identificados
con otra licencia o procedencia. Descarga el [texto completo](LICENSE.txt) y
los [avisos del proyecto](NOTICE.txt).

Se permite utilizar, estudiar, modificar y redistribuir el código, incluso con
fines comerciales, cumpliendo AGPL-3.0. No se exige una autorización comercial
separada para ejercer esos derechos. Las versiones modificadas ofrecidas por
red deben ofrecer sus fuentes correspondientes a los usuarios conforme a la
sección 13. Conservar una copia privada para desarrollo no exime de las
obligaciones al distribuir o prestar el programa modificado por red.

La elección se aplica a esta publicación y las posteriores que la conserven;
no se reescriben las etiquetas ni los avisos de las entregas históricas. Los
componentes de terceros mantienen sus licencias. El titular debe tener derecho
a licenciar las aportaciones originales incorporadas.

## Fuentes públicas del motor

El [paquete público YunoHost](https://github.com/Gente-del-Condado/headscale-gateway_ynh)
incluye `payload/source.tar.gz`, con las mismas fuentes de compilación que
`source.tar.gz` del complemento Home Assistant. Ambos incluyen `LICENSE`,
`NOTICE`, `VERSION`, `go.mod`, `go.sum` y `SOURCE.md`, con las órdenes Go para
compilar sin descargar módulos usando el `vendor/` incluido. Las sumas de los archivos se registran en `build-manifest.json`; el
publicador rechaza fuentes que no coincidan. Los avisos de las dependencias
acompañan al paquete en `notices/third_party/`.

Conserva el archivo de fuentes correspondiente a cada binario redistribuido.
Una rama `main` o un enlace al código más reciente no sustituye los fuentes de
una entrega anterior. Los archivos privados de estado, identidades VPN y claves
no forman parte de las fuentes del programa ni se publican.

## Inventario reproducible

`src/third_party/inventory.json` contiene los módulos de Go y paquetes npm encontrados. `tools/audit_licenses.py` cruza los módulos con las importaciones realmente enlazadas en servidor/helper para Linux, Windows y macOS, amd64 y arm64. Copia los textos de licencia y avisos sin modificarlos, incluyendo avisos anidados y el runtime Go.

Resultado de esta revisión: **737 módulos en el grafo Go; 78 módulos enlazados por el servidor/helper en alguna plataforma; 52 registros npm encontrados**. Los módulos del grafo no equivalen a código distribuido: hay dependencias de pruebas y herramientas. Las etiquetas `license_hints` son búsquedas heurísticas, no una declaración SPDX definitiva. `REVIEW` puede proceder de un archivo NOTICE o PATENTS que acompaña una licencia reconocida.

Componentes principales revisados:

| Componente | Texto observado | Consecuencia para esta entrega |
| --- | --- | --- |
| Tailscale / tsnet | BSD-3-Clause | Conservar copyright, condiciones y exención; no atribuir patrocinio |
| CertMagic y proveedor Technitium | Apache-2.0 | Conservar licencia y avisos pertinentes; revisar obligaciones de cambios y patentes |
| Go y módulos golang.org/x | BSD y avisos PATENTS | Conservar los textos correspondientes |
| modernc.org/sqlite | BSD-3-Clause; SQLite dominio público; avisos adicionales | Se incluyen LICENSE, LICENSE-SQLITE y LICENSE-3RD-PARTY.md completos |
| Electron | MIT para Electron; numerosas licencias para Chromium y componentes | Conservar LICENSE y LICENSES.chromium.html de cada distribución |
| Herramientas npm | Licencias declaradas por cada paquete | Inventario separado; no confundir herramientas de empaquetado con dependencias del código de aplicación |

La referencia a GPL detectada en `modernc.org/sqlite/LICENSE-3RD-PARTY.md` está en el texto de licencias secundarias de MPL incluido en su inventario transitivo; **no demuestra que el controlador SQLite enlazado sea GPL**. Ese mismo documento distingue expresamente componentes enlazados, solo pruebas y materiales fuera de compilación. Las coincidencias en avisos agregados deben revisarse en contexto.

## Distribución del runtime de escritorio

Cada paquete de escritorio incorpora un directorio `legal/` con los avisos
originales de Electron/Chromium, los fuentes completos de Electron, Chromium
(incluido FFmpeg), Node y NAN, los parches de Electron, las instrucciones de
construcción y los fuentes del cliente de bandeja. Se conserva cada licencia
MPL/LGPL/MIT/BSD de terceros; la AGPL-3.0-only cubre el código propio.

El conjunto se fija en `packaging/desktop/electron-sources.lock.json`: Electron
45.0.0, Chromium 156.0.8078.48 y Node 24.22.0. La primera preparación descarga
unos 4,5 GB fuera de Git y los incorpora físicamente a cada distribución.
Los archivos originales del runtime se contrastan con el ZIP oficial; en
Windows se admite el cambio de iconos/metadatos PE manteniendo el código.
El manifiesto registra sus sumas y el generador de entregas rechaza archivos
alterados, avisos ausentes o fuentes distintos de los fijados. No se permite
redistribuir una carpeta antigua de `bin/` sin volver a prepararla.

La [guía de fuentes de escritorio](ESCRITORIO_LICENCIAS.md) explica cómo preparar
la entrega, aplicar los parches y sustituir una biblioteca FFmpeg compatible.
No se añaden restricciones de ingeniería inversa para depurar modificaciones.
Se comprueban los archivos distribuidos; no se afirma haber recompilado todo
Chromium/Electron ni obtenido una reproducción binaria exacta del runtime.

No se ha detectado una licencia principal GPL/AGPL en los 78 módulos enlazados
del servidor/helper al revisar sus textos principales. El inventario no es una
auditoría de patentes, marcas ni procedencia de cada línea.

## Fuentes consultadas

- Textos exactos locales: `src/third_party/go/`, `src/third_party/npm/`, `src/third_party/go-toolchain/` y `src/third_party/electron/`.
- [Licencia de Tailscale](https://github.com/tailscale/tailscale/blob/main/LICENSE), contrastada con la versión local fijada en src/go.mod.
- [Preguntas frecuentes oficiales GPL](https://www.gnu.org/licenses/gpl-faq.en.html): vender software libre es posible, pero sus derechos de redistribución no desaparecen por venderlo.
- [Licencia LGPL 2.1](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html), para revisar obligaciones del componente concreto y de su combinación.
- [Licencia MPL 2.0](https://www.mozilla.org/en-US/MPL/2.0/), para revisar obligaciones a nivel de los archivos cubiertos.

## Regenerar

```bash
go -C src mod download
(cd src/desktop && npm ci)
python3 tools/audit_licenses.py
```

Después de actualizar dependencias, repite el inventario y revisa los cambios antes de publicar otra versión. Los paquetes de servidor y GUI llevan los avisos de terceros. Las claves privadas del emisor y las licencias de clientes se excluyen de los paquetes de fuentes.
