# Git, actualizaciones y entregas

El proyecto se mantiene en Git con el código común en `src/`, las fuentes
Android en `android/src/` y los instaladores generados en `addons/` y `yunohost/`.
Para localizar un componente consulta [ESTRUCTURA.md](ESTRUCTURA.md).

## Abrir un clon de trabajo

Clona el repositorio autorizado con tu clave SSH. El siguiente ejemplo usa una
dirección ficticia que debes sustituir por la de tu servidor.

```bash
git clone ssh://git@forgejo.example.net:2222/usuario/headscale-gateway.git
cd headscale-gateway
git status
python3 tools/check_repository.py
python3 tools/prepare_git_install.py --check
sha256sum -c CONTENIDO.sha256
```

Los requisitos de compilación están en el [README](../README.md). Para instalar
los paquetes ya incluidos, sigue [INSTALACION_PRIVADA.md](INSTALACION_PRIVADA.md).

## Trabajar sobre una versión nueva

Con el árbol limpio, descarga los cambios antes de empezar.

```bash
git pull --ff-only
```

Edita las fuentes y prepara los contenidos generados cuando corresponda. Revisa
las diferencias y ejecuta las pruebas indicadas en [CONTRIBUTING.md](../CONTRIBUTING.md).
Los commits nuevos conservan la historia de las versiones publicadas.

```bash
git diff
git add .
git ls-files -z -- . ':(exclude)CONTENIDO.sha256' | xargs -0 sha256sum > CONTENIDO.sha256
git add CONTENIDO.sha256
git diff --cached --stat
git commit -m "Describir el cambio realizado"
git push origin main
```

`CONTENIDO.sha256` contiene las sumas de los archivos versionados del árbol
entregado. Añade primero los archivos nuevos y después calcula las sumas.
Revisa el contenido preparado antes de confirmar el commit.

## Identificar una entrega

La versión del motor se define en `VERSION` y se cambia con
`python3 tools/set_version.py X.Y.Z`. Regenera después los paquetes instalables.
Una etiqueta Git identifica el commit de una entrega. El nombre debe ser nuevo
y responder a la política de versiones acordada.

```bash
# Tras fusionar el commit aprobado de 1.4.7 en main.
git tag -a v1.4.7 -m "Headscale Gateway 1.4.7"
git push origin refs/tags/v1.4.7
git ls-remote origin refs/heads/main 'refs/tags/v1.4.7^{}'
```

Las dos referencias de una entrega recién etiquetada deben resolver al commit
que acabas de publicar. La primera prueba documental utilizó `v0.1.0.1` mientras
el motor declaraba `1.4.4`. Consulta ambos valores antes de automatizar números
de versión en instaladores.

### Adjuntos desde el runner de Forgejo

En el runner de este proyecto, situado en el mismo YunoHost que Forgejo, una
etiqueta `vX.Y.Z` ejecuta primero seguridad y pruebas. Después publica los
adjuntos preparados bajo `/var/lib/hgw-release-staging/vX.Y.Z/`. No instala
el Gateway ni cambia servicios. Los envíos ordinarios a `main` conservan la
sincronización automática con GitHub, Mis-app, el paquete YunoHost y el manual.

El administrador prepara esa carpeta y sus archivos como propiedad de root,
con lectura para el usuario del runner y sin escritura de grupo u otros.
`release-assets.json` declara `tag`, `source_commit`, `body` (notas revisadas)
y una lista `assets` de objetos con `name`, `bytes` y `sha256`. El commit debe
coincidir exactamente con la etiqueta, que debe pertenecer al `main` aprobado. No se admiten enlaces,
archivos adicionales, nombres repetidos, rutas dentro de los nombres ni partes
de más de 2.100.000.000 bytes. La carpeta y sus antecesores deben estar protegidos.

`tools/publish_forgejo_release.py` utiliza únicamente el token temporal del
trabajo y la API local de Forgejo en `127.0.0.1:6000`. Mantiene la entrega en
borrador hasta descargar cada adjunto y comprobar su tamaño y SHA-256. Un error
detiene la publicación. Una repetición reutiliza solo adjuntos que se verifican;
no elimina archivos ni sobrescribe una release publicada. La API de Forgejo 17
devuelve todos los adjuntos en una consulta; este recurso no se pagina.
No guarda tokens ni
los envía a direcciones indicadas en las respuestas HTTP.

Los escritorios incluyen los fuentes y avisos completos del runtime. Si el
archivo supera el límite de adjuntos, divídelo con `tools/split_release.py` y
distribuye todas sus partes, sus sumas y las instrucciones de reconstrucción.
Preparar la carpeta antes de enviar la etiqueta evita un trabajo fallido por
falta de archivos. Este recorrido necesita el runner local; otro runner debe
adaptar explícitamente su almacenamiento y acceso a Forgejo.

Antes de enviar la etiqueta, se puede ejecutar manualmente «Calidad
multiplataforma» sobre `main`, indicando la etiqueta del borrador. Repite
seguridad y pruebas y usa `--draft-only`: sube y verifica los archivos, pero
no publica la release ni crea la etiqueta Git. Solo después de confirmar
`BORRADOR_VERIFICADO_SIN_PUBLICAR` se envía la etiqueta; el trabajo automático
revalida los adjuntos y publica. Esta preparación manual no sustituye ni
dispara los flujos automáticos de Mis-app y GitHub Pages.

Si falla exclusivamente la infraestructura de publicación, corrige el publicador
mediante una solicitud revisada y fusionada en Forgejo. Después ejecuta el flujo
sobre `main` con la etiqueta original y «Publicar la etiqueta existente» activado.
Repite seguridad, pruebas y descarga de todos los adjuntos. Conserva la etiqueta
y los paquetes originales: solo permite cambios posteriores en el publicador,
su flujo, sus pruebas, esta guía y `CONTENIDO.sha256`. Rechaza cambios en el
producto o una etiqueta que no sea antecesora del `main` ejecutado. La casilla
permanece desactivada por defecto para preparar borradores sin publicarlos.

Forgejo puede convertir automáticamente un borrador en release al recibir la
etiqueta Git, antes de que termine el flujo. Si faltan adjuntos, la publicación
por etiqueta permite recuperar ese borrador únicamente después de verificar que
todos los adjuntos existentes pertenecen al manifiesto protegido y sus SHA-256
coinciden. Después sube los que faltan, verifica todas las descargas y publica.
Para un reintento manual activa también «Recuperar como borrador una release
incompleta». Sin esta opción, el publicador no añade archivos a una release
publicada. Una release completa, con archivos ajenos o con sumas distintas no
se devuelve a borrador. La recuperación no cambia ni recrea la etiqueta.

## Exportar una carpeta o un ZIP limpio

```bash
python3 tools/prepare_git_install.py --check
python3 tools/repository_archive.py --output releases/headscale-gateway-fuentes.zip
```

El exportador utiliza una lista de fuentes permitidas y verifica los paquetes.
Cada ZIP incorpora su propia lista `CONTENIDO.sha256`. Con `--include-releases`
añade los archivos Android autorizados de `releases/android/`, si existen y su
suma coincide.

Descomprime las entregas en una carpeta nueva y comprueba sus sumas antes de
usarlas. Si trabajas sobre un clon existente, incorpora los cambios como una
modificación revisada que conserve su directorio `.git` y su historia.

## Datos de las instalaciones

Las fuentes del proyecto y los datos de ejecución tienen ubicaciones propias.
En Home Assistant los datos del motor están en `/data/gateway/`. En YunoHost se
utiliza la subcarpeta `gateway/` del directorio de datos asignado a la instancia.
Mantén las bases de datos, claves, perfiles y copias privadas fuera del clon.
Las actualizaciones se realizan con el procedimiento de cada plataforma.
