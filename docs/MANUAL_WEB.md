# Editar y publicar el manual web

El manual se genera con MkDocs a partir de Markdown. Los textos se pueden leer
en Forgejo, GitHub o un editor. El resultado es una carpeta HTML estática con
navegación, búsqueda y capturas. Sus recursos se sirven desde la misma carpeta.

## Preparar el entorno

Desde la raíz del proyecto, crea un entorno virtual en la carpeta de resultados.

```bash
python3 -m venv builds/docs-env
builds/docs-env/bin/python -m pip install -r requirements-docs.txt
builds/docs-env/bin/python -m mkdocs build --strict
python3 tools/check_manual.py builds/manual
```

En Windows, el ejecutable del entorno es `builds\docs-env\Scripts\python.exe`.
La versión de MkDocs está fijada en `requirements-docs.txt`. Las dependencias de
la documentación se instalan en ese entorno independiente.

Abre `builds/manual/index.html` en el navegador. Para revisar cambios en vivo,
arranca el servidor local de MkDocs.

```bash
builds/docs-env/bin/python -m mkdocs serve --dev-addr 127.0.0.1:8000
```

Visita `http://127.0.0.1:8000/`. Usa Ctrl+C para detener el servidor. La web también
se puede copiar completa a cualquier alojamiento estático, incluso bajo una
subruta. La búsqueda utiliza un índice local y funciona al abrir los HTML.

## Dónde editar cada cosa

| Cambio | Archivo fuente |
| --- | --- |
| Portada y recorrido de inicio | `docs/index.md`, `docs/PRIMEROS_PASOS.md` |
| Manual funcional | `docs/USO_COMPLETO.md` y guías específicas de `docs/` |
| Complemento Home Assistant | `packaging/homeassistant/DOCS.md` |
| Integración de entidades | `packaging/hacs/README.md` |
| YunoHost | `packaging/yunohost/doc/ADMIN.md` y `doc/` |
| Android | `android/README.md`, `docs/ANDROID.md` |
| Mapa y orden del menú | `mkdocs.yml` |
| Colores, espacios y adaptación móvil | `tools/manual_theme/assets/manual.css` |
| Plantilla HTML | `tools/manual_theme/main.html` |
| Búsqueda y menú móvil | `tools/manual_theme/assets/manual.js` |
| Incorporación de fuentes e índice | `tools/manual_hooks.py` |
| Guía para programadores e IA | `CONTRIBUTING.md`, `AGENTS.md` |

Las páginas virtuales de plataforma se crean durante la compilación. La web y
el paquete instalado toman su contenido de la misma fuente. Los TXT utilizados
por los paquetes se presentan también como páginas navegables.

Al añadir una guía, crea el Markdown, incorpora su entrada a `nav` en
`mkdocs.yml` y enlázala desde el recorrido correspondiente. Usa enlaces relativos,
textos alternativos en las imágenes y un solo título de primer nivel por página.
Las capturas se mantienen según [CAPTURAS.md](CAPTURAS.md).

## GitHub Pages

El manual común se publica en los dos repositorios públicos de distribución:

- [Manual de YunoHost](https://gente-del-condado.github.io/headscale-gateway_ynh/),
  con su [capítulo de instalación](https://gente-del-condado.github.io/headscale-gateway_ynh/YUNOHOST.html).
- [Manual de Home Assistant](https://gente-del-condado.github.io/headscale-gateway-ha/),
  con los capítulos del [complemento](https://gente-del-condado.github.io/headscale-gateway-ha/HOME_ASSISTANT.html)
  y de la [integración de entidades](https://gente-del-condado.github.io/headscale-gateway-ha/INTEGRACION_HA.html).

Las webs reutilizan el manual existente, sin mantener otras fuentes. El flujo
**Manual web** compila y comprueba los enlaces; con `PUBLISH_PUBLIC_DOCS=true`
en las variables Actions del proyecto principal, publica el resultado validado
en la rama `gh-pages` de ambos repositorios de Forgejo. Sus espejos deben
replicar también esa rama a GitHub. En cada destino, **Settings → Pages** usa
**Deploy from a branch → gh-pages → / (root)**. Los cambios aprobados que llegan
a `main` mediante la réplica vuelven a publicar automáticamente; los PR solo
compilan y validan, sin acceder a las claves.

El publicador usa `YNH_FORGEJO_DEPLOY_KEY` para YunoHost y una clave dedicada
`DOCS_HA_FORGEJO_DEPLOY_KEY` con escritura limitada al repositorio
`headscale-gateway-ha` de Forgejo. No necesita permisos de escritura en el
proyecto principal. `tools/publish_manual.py` exige otro checkout en `gh-pages`
y conserva su historial: nunca modifica el `main` del paquete. Solo publica
los HTML y recursos del manual generado, con `.nojekyll`; no copia las fuentes
del motor ni los ejecutables de los paquetes.

El repositorio principal permanece privado. Si su plan de GitHub no permite
Pages privados, se utiliza esta distribución del HTML mediante los repositorios
públicos. Publicar el manual lo hace accesible sin autenticación.

### Alternativa: Pages en el propio repositorio principal

El repositorio contiene `.github/workflows/pages.yml`. La compilación se ejecuta
en las solicitudes de cambio y comprueba enlaces. La publicación requiere que
habilites GitHub Pages en el repositorio que quieras usar para el manual.

1. Crea o utiliza el repositorio de GitHub autorizado para alojar esta documentación.
2. Sube el proyecto allí conservando el remoto de Forgejo que utilizas para desarrollar.
3. En **Settings → Pages → Build and deployment**, selecciona **GitHub Actions**.
4. En **Settings → Secrets and variables → Actions → Variables**, crea la variable
   de repositorio `PUBLISH_DOCS` con el valor `true`.
5. En **Actions → Manual web**, ejecuta **Run workflow** sobre `main`.
6. Abre la URL que muestre el despliegue y comprueba Inicio, búsqueda y una guía de plataforma.

Los siguientes cambios en la documentación de `main` volverán a publicar el
manual mientras esa variable esté habilitada. Revisa la visibilidad que ofrece
el plan de GitHub antes de activar el alojamiento. La disponibilidad de Pages
en repositorios privados depende del plan y del tipo de organización.

El workflow usa los permisos de `contents: read`, `pages: write` e `id-token: write`
en los trabajos que los necesitan. La construcción usa únicamente archivos de
este repositorio y dependencias del generador. El resultado publicado es
`builds/manual/`.

La configuración sigue el [flujo oficial de GitHub Pages](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

## Alojarlo junto a Forgejo

El mismo resultado se puede servir desde un directorio estático configurado por
el administrador del servidor. Primero ejecuta la compilación y comprueba la
carpeta `builds/manual/`. Publica su contenido conservando las rutas relativas.
Si el manual es privado, configura la autenticación del alojamiento antes de
compartir la dirección. El acceso al repositorio y el acceso a esa web tienen
su configuración propia.

## Comprobaciones de una actualización

```bash
builds/docs-env/bin/python -m mkdocs build --strict
python3 tools/check_manual.py builds/manual
```

Comprueba también el aspecto en un navegador de escritorio y uno estrecho,
el menú mediante teclado, una búsqueda y los ejemplos. Las pruebas de enlaces
verifican los archivos y fragmentos internos del HTML generado. Las referencias
a servicios externos deben revisarse cuando cambien los procedimientos que describen.
