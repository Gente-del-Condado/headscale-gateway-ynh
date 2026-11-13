# Continuar el proyecto

Esta guía es el punto de entrada para programadores y asistentes de IA. Lee
`README.md`, `docs/ESTRUCTURA.md` y la guía del componente antes de modificarlo.
Comprueba `git status`, el último commit y las instrucciones locales al empezar.

## Qué estamos manteniendo

Headscale Gateway comparte un motor Go entre instalación directa, Home
Assistant, YunoHost y Android. El motor, el panel y el cliente de bandeja viven
en `src/`. Android adapta el ciclo de vida y el túnel desde `android/src/`.
El proyecto se desarrolla principalmente en español.

El código propio tiene la licencia indicada en `LICENSE`. Conserva los avisos,
licencias, nombres y procedencia de terceros. Las dependencias originales se
mantienen en su idioma. `src/third_party/` contiene avisos, no un vendor de Go.

## Dónde editar

| Trabajo | Fuente | Comprobación principal |
| --- | --- | --- |
| Arranque y recarga | `src/main.go`, `startup_*.go`, `runtime_*.go` | Pruebas Go y despliegue aislado |
| Proxy y DNS | `src/proxy*.go`, `src/dns_*.go` | Pruebas de transporte, propiedad y limpieza DNS |
| Panel y API | `src/internal/admin/` | Go, pruebas JS, referencia e i18n |
| SQLite y secretos | `src/internal/state/` | Restauración, concurrencia y migraciones |
| Copias y perfiles | `src/internal/securebackup/`, `src/internal/configtransfer/`, `src/admin_backup_restore_*` | Integridad, permisos y compatibilidad |
| Integración Home Assistant | `custom_components/headscale_gateway/` | Suite `tests/homeassistant/` |
| Complemento Home Assistant | `packaging/homeassistant/` | Pruebas Python y contenedor |
| Paquete YunoHost | `packaging/yunohost/` | Pruebas Python, nginx/NSS y aceptación en VM |
| Android | `android/src/`, `src/mobile_android.go` | Motor de ambas arquitecturas, Lint e instrumentación |
| Bandeja | `src/desktop/` | `npm test` y pruebas visuales específicas |
| Manual | `docs/`, documentación de plataforma, `mkdocs.yml` | Compilación estricta y enlaces HTML |

`packaging/homeassistant/config.yaml.in` es una plantilla. El generador la
convierte en `addons/headscale_gateway/config.yaml`. Mantén el sufijo `.in` para
que Home Assistant descubra un único complemento dentro del repositorio.

`addons/` y `yunohost/` se generan y están versionados porque se instalan desde
Git privado. Cambia la fuente en `packaging/` o `src/` y ejecuta
`python3 tools/prepare_git_install.py`. El manifiesto comprueba ambos árboles.

## Contratos que debe respetar cualquier cambio

- SQLite es la fuente de configuración persistente. El archivo de arranque
  aporta opciones del proceso y valores iniciales del instalador.
- Actualizar conserva credenciales, MFA, identidad VPN, claves y datos.
- Un solo proceso utiliza cada base de datos y cada identidad tsnet.
- El panel, la API y los formatos de copia son comunes a todas las plataformas.
- La sesión web, el rol, los alcances Bearer y CSRF tienen comprobaciones distintas.
  Una IP loopback o una cabecera recibida por sí sola no acredita un administrador.
- DNS solo elimina registros dentro del ámbito de propiedad del Gateway.
- La restauración valida la carpeta temporal antes de programar su limpieza.
- Los transportes del proxy conservan la escritura del cuerpo en respuestas 101.
- Los errores y registros excluyen tokens, contraseñas y claves privadas.
- La comprobación de i18n es de solo lectura. Extraer y comprobar son órdenes separadas.

## Cómo escribir y documentar

Usa nombres descriptivos, funciones pequeñas y los componentes existentes.
Escribe los comentarios del código propio en español. Explica la finalidad,
las precondiciones, los límites de confianza y las decisiones de concurrencia
cuando aporten información que el código no muestre por sí solo. Conserva los
identificadores públicos y las directivas de herramientas como `//go:build`.

Documenta nuevas rutas, parámetros, migraciones, formatos y cambios de uso en
la misma modificación. Añade pruebas cuando el comportamiento nuevo o el fallo
corregido las necesite. Para un cambio solo documental comprueba el manual,
los enlaces y las sumas, sin inventar resultados de ejecución del producto.

Los textos de Home Assistant y YunoHost se editan en `packaging/`. El manual
web los incorpora automáticamente desde allí. La configuración de navegación
está en `mkdocs.yml`, la presentación en `tools/manual_theme/` y la integración
con documentos externos en `tools/manual_hooks.py`.

## Secuencia de trabajo

1. Revisa el estado y los cambios ajenos. Delimita los archivos que vas a editar.
2. Lee el recorrido normal del componente y sus pruebas existentes.
3. Modifica las fuentes y actualiza sus comentarios y documentación.
4. Si cambian rutas o mensajes, ejecuta los generadores de referencia e i18n.
5. Si cambian fuentes del motor o plantillas, regenera los paquetes instalables.
6. Ejecuta las comprobaciones correspondientes y registra sus resultados reales.
7. Actualiza `CONTENIDO.sha256` al preparar los archivos del commit.
8. Guarda un commit nuevo. Sube por SSH cuando esté autorizado y verifica las referencias.

```bash
python3 -B tools/reference_catalog.py
python3 -B tools/extract_i18n.py
python3 -B tools/check_i18n.py
python3 -B tools/prepare_git_install.py
bash tools/verify.sh
```

Para el manual, instala `requirements-docs.txt` en un entorno virtual y ejecuta
`python -m mkdocs build --strict`, seguido de
`python3 tools/check_manual.py builds/manual`. Consulta `docs/MANUAL_WEB.md`.

## Versiones y publicación

`VERSION` es la versión del motor y de los paquetes. Se gestiona con
`python3 tools/set_version.py X.Y.Z`. El proyecto utiliza tres componentes
numéricos para los manifiestos. Las etiquetas Git identifican commits.
`v0.1.0.1` fue una prueba de actualización documental, con motor `1.4.4`.
Elige una política de versiones explícita antes de cambiar esa numeración.

Las etiquetas ya publicadas pertenecen a su commit original. Los cambios
posteriores se guardan en commits nuevos. Comprueba la rama, el remoto y la
etiqueta antes de publicar. El repositorio se sube por SSH. El manual de GitHub
Pages se habilita por separado siguiendo `docs/MANUAL_WEB.md`.

## Limpieza y datos de prueba

Genera compilaciones en `bin/`, `builds/`, `releases/` o una carpeta temporal.
Mantén cachés y entornos virtuales fuera de las fuentes. Revisa qué contiene
un directorio antes de borrarlo. Los binarios de `yunohost/payload/` y el archivo
`addons/headscale_gateway/source.tar.gz` son necesarios para los instaladores.

Mantén bases de datos, perfiles, claves, credenciales y datos de instalaciones
fuera de Git. Utiliza nombres y direcciones ficticios en pruebas y capturas.
Antes de finalizar, revisa diferencias, archivos generados y estado de Git.
Explica qué se verificó, qué queda pendiente y cómo repetir la comprobación.
