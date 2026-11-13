# Desarrollar Headscale Gateway

Las aportaciones originales se distribuyen bajo `AGPL-3.0-only`, igual que el
proyecto. Aporta únicamente materiales que tengas derecho a licenciar y conserva
los avisos de terceros. Consulta [licencias](docs/LICENCIAS.md).

El motor, SQLite, la autenticación, los servicios, los certificados y las copias
son comunes a todas las instalaciones. Empieza por [AGENTS.md](AGENTS.md), el
[mapa de carpetas](docs/ESTRUCTURA.md) y la [arquitectura](docs/ARQUITECTURA.md).

## Preparar el entorno

Trabaja desde la raíz del repositorio. Usa la versión Go indicada en
`src/go.mod`, Python 3 y Node.js para las comprobaciones. La bandeja necesita
las dependencias fijadas en `src/desktop/package-lock.json`. Android tiene
requisitos adicionales descritos en [su guía](android/README.md).

```bash
git status --short
python3 tools/check_repository.py
python3 tools/set_version.py --check
bash tools/verify.sh
```

La primera ejecución de Go puede descargar módulos. Instala las dependencias
de documentación en un entorno virtual separado siguiendo
[MANUAL_WEB.md](docs/MANUAL_WEB.md). No guardes credenciales ni datos productivos
para preparar el entorno. Las pruebas usan directorios temporales.

## Elegir la fuente correcta

| Cambio | Fuente que debes editar |
| --- | --- |
| Servidor, VPN, DNS, proxy | `src/` y sus paquetes `internal/` |
| Panel, sesión web y API | `src/internal/admin/` |
| Integración de sensores | `custom_components/headscale_gateway/` |
| Lanzador Home Assistant | `packaging/homeassistant/` |
| Instalación y mantenimiento YunoHost | `packaging/yunohost/` |
| Contrato inicial de todos los instaladores | `packaging/common/hgw_bootstrap.py` |
| Manual y capturas | `docs/` y los documentos originales de cada plataforma |

Los directorios `addons/` y `yunohost/` son resultados versionados para instalar
por Git privado. No los edites a mano. La plantilla Home Assistant conserva el
nombre `config.yaml.in`; el generador crea el `config.yaml` instalable.

```bash
python3 tools/prepare_git_install.py
python3 tools/prepare_git_install.py --check
```

El manifiesto `git-install-manifest.json` vincula fuentes y resultados. Guarda
los paquetes regenerados y el manifiesto en el mismo cambio que sus fuentes.

## Mantener los contratos

Las rutas internas son `/admin/`, `/api/v1/…` y las rutas públicas existentes.
El manejador HTTP común resuelve el prefijo de instalación. Una plataforma no
debe duplicar manejadores ni autenticación. La delegación `proxy_auth` verifica
proxy, identidad y grupo administrativo en cada petición.

SQLite conserva la configuración funcional. El archivo de arranque aporta
opciones del anfitrión y valores iniciales. Una actualización conserva
credenciales, MFA, identidad VPN y datos. Nunca se comparte la misma base o
identidad entre dos procesos activos.

Escribe comentarios informativos en español para explicar precondiciones,
propiedad de recursos, concurrencia y límites de confianza. Conserva los nombres
públicos, las directivas de compilación y los avisos originales de terceros.
Evita comentarios que solo repitan la instrucción siguiente.

## Documentar y comprobar un cambio

Si añades rutas o mensajes, actualiza sus catálogos antes de verificar:

```bash
python3 tools/reference_catalog.py
python3 tools/extract_i18n.py
python3 tools/check_i18n.py
```

La extracción escribe archivos; `--check` y la comprobación de traducciones son
de solo lectura. Edita las traducciones humanas en `src/locale/`.

| Área modificada | Comprobaciones |
| --- | --- |
| Motor, panel o empaquetado | `bash tools/verify.sh` |
| Recuperación o inicio interactivo | `python3 tools/check_console.py` |
| Despliegue, proxy o persistencia | `python3 tools/check_deployment.py` |
| Concurrencia Go | `CGO_ENABLED=1 go -C src test -race ./internal/admin .` |
| Home Assistant y YunoHost | Suites y aceptación descritas en [PLATAFORMAS.md](docs/PLATAFORMAS.md) |
| Manual | Compilación estricta y `python3 tools/check_manual.py builds/manual` |
| Android | [Compilación y validación Android](android/README.md) |

No atribuyas a pruebas aisladas resultados de instalación en equipos reales.
Documenta los límites y los pasos pendientes. Los cambios de lógica necesitan
pruebas del comportamiento, incluidos errores y conservación de datos.

Los flujos de calidad de GitHub y Forgejo ejecutan **Seguridad de dependencias**
en cada `push` y `pull_request`, sin filtros de archivos. El checkout corresponde
al evento del cambio, sin fijarlo a `main`; en los pull requests se analiza el
código que prepara cada plataforma para ese evento. Este trabajo se ejecuta
en paralelo a las pruebas y no usa secretos de publicación.

Se aplican las mismas auditorías previas a Mis-app y al paquete YunoHost: `govulncheck@v1.9.0`
a nivel de módulo y de llamadas Go, instalación npm sin scripts y
`npm audit --include=dev --audit-level=low`. Una vulnerabilidad o un error del
análisis deja el trabajo fallido. Los comandos para repetirlo están en
[publicación en Mis-app](docs/USO_COMPLETO.md#publicar-el-complemento-en-mis-app).
El [publicador YunoHost](docs/USO_COMPLETO.md#publicar-el-paquete-yunohost)
exporta exclusivamente `yunohost/` a la raíz de su repositorio de distribución,
después de validar los manifiestos y los binarios de ambas arquitecturas.

Para impedir una fusión con ese resultado fallido, la protección de `main`
en cada servidor debe exigir el estado **Seguridad de dependencias**. Declarar
el trabajo en YAML proporciona el resultado del análisis; no cambia las reglas
de protección del repositorio. Los PR de colaboradores externos pueden necesitar
la aprobación de ejecución exigida por el servidor.

El runner de Forgejo debe estar en línea y anunciar la etiqueta
`ubuntu-latest`. El trabajo de pruebas prepara Python 3.12 mediante la acción
oficial `astral-sh/setup-uv`, fijada a un commit, y `uv` 0.13.7. El intérprete
y el entorno virtual se instalan en `RUNNER_TEMP`, sin APT ni privilegios root
ni cambios en el Python del servidor YunoHost. Se necesita acceso a las
publicaciones oficiales de Astral y a PyPI para las dependencias del manual.
Forgejo no admite el campo `permissions` de GitHub: sus permisos se gestionan
en el servidor y los PR externos quedan sujetos a sus restricciones propias.

La protección de `main` en Forgejo exige el contexto exacto
`Calidad multiplataforma / Seguridad de dependencias (pull_request)`, también
para administradores, y deshabilita los pushes directos. Las reglas de GitHub
para repositorios privados de una organización requieren un plan que permita
aplicarlas (Team o Enterprise); crear una regla con un plan incompatible no
proporciona ese bloqueo.

## Versiones, commits y entregas

`VERSION` es la versión común de motor y paquetes, en formato `X.Y.Z`. Se cambia
con `python3 tools/set_version.py X.Y.Z`. Una etiqueta Git identifica un commit;
la etiqueta documental `v0.1.0.1` mantiene el motor `1.4.4`.

Antes de guardar un commit, revisa el diff y añade los archivos intencionados.
La suma se genera después de preparar el índice para incluir los archivos nuevos
y excluir los eliminados:

```bash
git add --all
git diff --cached --stat
git ls-files -z -- . ':(exclude)CONTENIDO.sha256' | xargs -0 sha256sum > CONTENIDO.sha256
git add CONTENIDO.sha256
sha256sum --check --quiet CONTENIDO.sha256
git commit -m "Describir el cambio realizado"
```

La [guía Git](docs/MIGRACION_REPOSITORIO.md) explica SSH, subida de `main`,
etiquetas nuevas y exportación. No reemplaces etiquetas publicadas ni borres
historial para limpiar archivos. La [guía del manual](docs/MANUAL_WEB.md) explica
la publicación independiente en GitHub Pages.
