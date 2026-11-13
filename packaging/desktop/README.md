# Fuentes y avisos de la versión de escritorio

Cada distribución Windows x64 y Linux x64 de Gateway-Tray incluye `legal/`.
Conserva esa carpeta completa junto con el ejecutable al redistribuirlo.

## Qué incluye

- `LICENSE` y `LICENSES.chromium.html`: avisos originales del runtime concreto,
  sin sustituirlos por la licencia de Headscale Gateway.
- `sources/`: archivos completos de Electron, Chromium, Node y NAN. Chromium
  incluye FFmpeg, Blink y las dependencias incorporadas con sus propios avisos.
  Electron incluye sus parches, `DEPS`, configuraciones GN y scripts de construcción.
- `application-source/`: fuentes del cliente Gateway-Tray, lockfile npm, scripts
  de empaquetado y licencia AGPL-3.0-only del código propio.
- `manifest.json`: versiones, procedencia HTTPS, sumas de archivos originales y
  relación de todos los archivos del runtime y del conjunto de fuentes.

La versión fijada es Electron **45.0.0**, commit
`54f961b44b83e4ece81065ab60a182fd0c38c01e`, Chromium **156.0.8078.48** y
Node **24.22.0**. El lock de este directorio fija también las sumas de las
distribuciones oficiales Linux/Windows. Las sumas de sus ZIP se contrastaron
con `SHASUMS256.txt` de Electron. El archivo Chromium se contrastó además con
el MD5 publicado en los metadatos HTTPS del almacenamiento oficial de Chromium.

No se han cambiado los fuentes de Electron, Chromium ni FFmpeg en el proyecto.
El empaquetador renombra el ejecutable y añade la aplicación. En Windows puede
actualizar iconos y metadatos PE; se comprueba que su sección `.text` coincide
con el ejecutable oficial. El resto de archivos originales conservados se
compara byte a byte con el ZIP oficial, incluidos FFmpeg y ambos avisos.

## Preparar una distribución

Desde la raíz del proyecto, instala las dependencias de `src/desktop/` y ejecuta:

```bash
cd src/desktop
npm ci
npm run pack:all
```

Se necesita Python 3.11 o posterior. La primera ejecución descarga unos 4,5 GB
de fuentes y los ZIP oficiales en `builds/desktop-license-cache/`. Los siguientes
empaquetados reutilizan esa caché después de comprobar sus sumas. Los archivos
grandes son artefactos de distribución y no se guardan en el historial Git.

`pack.cjs` prepara y verifica `legal/` antes de sustituir una entrega existente
en `bin/`. `tools/package_release.py` vuelve a comprobar todos los archivos y
rechaza escritorios antiguos, fuentes ausentes o archivos alterados. Los ZIP
Windows y los TAR Linux incluyen los fuentes, no solo un enlace ni una oferta
de suministro futuro. Distribuye esos paquetes completos y conserva sus sumas.
No cambies únicamente la versión de Electron en npm: actualiza este lock,
revisa las dependencias y los avisos de la nueva versión y reconstruye ambos
escritorios antes de distribuirlos.

Para generar exclusivamente las entregas de escritorio ya preparadas:

```bash
python3 tools/package_release.py --desktop-only --output releases/escritorio
```

El directorio de salida debe estar vacío. Cada entrega ocupa más de 4 GB y
requiere una herramienta con soporte ZIP64/PAX. La suma del archivo final se
publica en `SHA256SUMS` junto con el resto de artefactos de esa entrega.

## Compilar o modificar Electron y FFmpeg

Los archivos de `sources/` son fuentes base más los parches de Electron.
La guía completa de construcción y los requisitos Linux/Windows están dentro
del archivo Electron, en `docs/development/build-instructions-gn.md` y las guías
de cada plataforma. Los parches se enumeran en `patches/config.json`; los de
FFmpeg están en `patches/ffmpeg/`. No compiles únicamente una versión genérica
de FFmpeg: utiliza la revisión incluida en Chromium y los parches de Electron.

Para reconstruir el árbol, extrae Chromium como `src/`, Electron como
`src/electron/`, Node como `src/third_party/electron_node/` y NAN como
`src/third_party/nan/`, quitando el directorio superior de cada archivo.
Conserva los fuentes restantes de Chromium. Sigue el flujo manual de la guía
archivada para instalar depot_tools y las herramientas/SDK de la plataforma.
El script `script/apply_all_patches.py` de Electron aplica la lista de parches
desde el directorio padre de `src/`; requiere inicializar los repositorios Git
indicados en `patches/config.json`. No omitas esos parches ni cambies `DEPS`.
Las configuraciones oficiales están en `build/args/` del archivo Electron.
Las herramientas, SDK y espacio de compilación no están incluidos: este
conjunto proporciona los fuentes y cambios, no un entorno de compilación
preinstalado. No se afirma reproducibilidad binaria byte a byte de Electron.

También puedes seguir la sincronización Git de la guía oficial fijando el
checkout Electron al commit indicado arriba; `DEPS` fija el resto de revisiones.
El proyecto valida la procedencia de los binarios oficiales y el contenido de
los paquetes, pero no recompila Chromium/Electron completo durante su CI.

FFmpeg se usa bajo LGPL-2.1-or-later, con los avisos exactos que recoge
`LICENSES.chromium.html`; sus fuentes y cambios están en los archivos incluidos.
Puedes modificar el programa, depurarlo y sustituir `libffmpeg.so` en Linux o
`ffmpeg.dll` en Windows por una compilación de interfaz compatible. Detén la
aplicación y conserva una copia antes de sustituir una biblioteca. El proyecto
no añade restricciones de ingeniería inversa para depurar esas modificaciones.
Las licencias MPL/LGPL de Chromium conservan sus términos y sus fuentes se
facilitan junto con el runtime; no se relicencian como AGPL.

Referencias oficiales:

- Electron: https://github.com/electron/electron/tree/v45.0.0
- Construcción: https://www.electronjs.org/docs/latest/development/build-instructions-gn
- FFmpeg: https://ffmpeg.org/legal.html
- LGPL 2.1: https://www.gnu.org/licenses/old-licenses/lgpl-2.1.html
- MPL 2.0: https://www.mozilla.org/en-US/MPL/2.0/
