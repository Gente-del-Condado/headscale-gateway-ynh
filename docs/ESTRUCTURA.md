# Estructura y mantenimiento

## Fuentes y responsables

| Directorio | Responsabilidad | Edición |
| --- | --- | --- |
| `src/*.go` | Orquestación común, configuración, servicios y DNS | Fuente principal |
| `src/internal/admin/` | Panel, API, sesiones, certificados y copias | Fuente principal |
| `src/internal/network/` | Contratos y llamadas Headscale | Fuente principal |
| `src/internal/state/`, `src/internal/securebackup/`, `src/internal/configtransfer/` | SQLite, cifrado y formatos compartidos | Fuente principal |
| `src/internal/pki/`, `src/internal/platform/` | Certificados y adaptación al sistema | Fuente principal |
| `src/desktop/` | Cliente de bandeja | Fuente principal |
| `src/tools/` | Programas auxiliares Go; mismo módulo | Fuente principal |
| `src/locale/` | Catálogos editables; español como idioma original | Fuente principal |
| `src/third_party/` | Avisos e inventario de licencias; no es vendor de Go | Conservar avisos originales |
| `android/src/main/` | Aplicación Android y JNI | Fuente principal |
| `android/src/androidTest/` | Instrumentación aislada | Solo pruebas |
| `android/third_party/` | Motor HEV y avisos | Conservar licencias y procedencia |
| `packaging/` | Lanzadores e instaladores de las plataformas | Fuente principal |
| `custom_components/` | Entidades Home Assistant | Fuente principal; ruta convencional |
| `addons/`, `yunohost/` | Instalación desde Git privado | Generado; no editar |
| `tools/` | Automatización de mantenimiento | Fuente principal |
| `tests/` | Pruebas Python, Home Assistant y contenedores | Fuente principal |
| `docs/`, `examples/` | Manuales, contratos y ejemplos ficticios | Fuente principal |
| `bin/`, `builds/`, `releases/` | Resultados de compilación y entrega | Ignorados por Git salvo el índice de releases |

El módulo conserva sus rutas de importación. Moverlo a `src/` no cambia sus
API ni la ubicación de los datos de una instalación. Android adapta lo que
exige el sistema operativo sin bifurcar servicios, PKI, API o formatos de copia.

## Flujo de cambio

1. Editar fuentes o plantillas; nunca parchear los paquetes generados.
2. Si cambia la versión, ejecutar `python3 tools/set_version.py X.Y.Z`.
3. Regenerar referencia y traducciones cuando cambien sus entradas:
   `python3 tools/reference_catalog.py` y `python3 tools/extract_i18n.py`.
   Verificar las traducciones con `python3 tools/check_i18n.py`.
4. Regenerar `addons/` y `yunohost/` con `python3 tools/prepare_git_install.py`.
5. Ejecutar `bash tools/verify.sh`. Validar Android si se ha modificado su capa.
6. Empaquetar en una carpeta nueva bajo `releases/`; revisar sus sumas.

`git-install-manifest.json` relaciona las fuentes con las carpetas instalables.
No se actualiza a mano. Detecta cambios en las fuentes y modificaciones o
faltas en los paquetes generados. El exportador exige que esté actualizado.

## Datos fuera del código

No guardar aquí bases SQLite, claves SSH, claves de firma, claves privadas de
CA, copias `.hgw`, perfiles `.hgwcfg`, credenciales Headscale ni estado tsnet.
Las pruebas emplean rutas temporales y la APK `.lab`; no usan los datos de la
aplicación habitual. Los ejemplos usan dominios e identidades ficticios.

## Dependencias

Go usa `src/go.mod` y `src/go.sum`. Electron usa
`src/desktop/package-lock.json`. Android declara SDK, NDK, AGP y JDK en
sus archivos de construcción y su guía. No se copian cachés ni dependencias
Node instaladas a las entregas de fuentes. El código HEV incluido en Android
sí es fuente necesaria y se conserva con sus licencias.

## Manual web y continuidad

`AGENTS.md` contiene instrucciones para continuar con otra IA o desarrollador.
`CONTRIBUTING.md` detalla el flujo de trabajo. `mkdocs.yml` define el índice del
manual y `requirements-docs.txt` fija su generador.

Los textos de plataforma se incorporan mediante `tools/manual_hooks.py` sin
mantener duplicados editables. La plantilla, estilos y búsqueda residen en
`tools/manual_theme/`. `tools/check_manual.py` comprueba enlaces y anclas del
resultado HTML. El resultado se genera en `builds/manual/`, que no se versiona.

La automatización de GitHub Pages está en `.github/workflows/pages.yml`;
requiere activar Pages y la variable de publicación. La comprobación en Forgejo
se integra en `.forgejo/workflows/quality.yml`. Ninguna instalación del Gateway
necesita MkDocs para funcionar.
