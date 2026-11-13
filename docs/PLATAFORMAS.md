# Una aplicación y sus integraciones

Headscale Gateway utiliza el mismo binario, panel, API, autenticación y SQLite
cuando se ejecuta directamente, en Home Assistant OS o en YunoHost. Los
instaladores configuran la conexión con el anfitrión; no seleccionan una versión
alternativa del servidor ni mantienen rutas distintas.

Para Forgejo privado con `git pull` y copia local, sigue
[Instalación privada](INSTALACION_PRIVADA.md). El repositorio incluye
`addons/headscale_gateway/` y `yunohost/` listos para instalar, sin ZIP.

## Estructura del repositorio

| Ubicación | Responsabilidad |
| --- | --- |
| `src/` y `src/internal/` | Servidor, panel y lógica funcional compartidos |
| `custom_components/headscale_gateway/` y `hacs.json` | Integración de entidades; copia manual desde Forgejo privado o exportación para Marketplace/HACS en GitHub público |
| `packaging/common/hgw_bootstrap.py` | Inicialización común a todos los instaladores |
| `packaging/homeassistant/` | Contenedor y conexión con Supervisor |
| `packaging/yunohost/` | Instalación, nginx, systemd, actualización y copias |
| `VERSION` | Versión común del servidor y sus paquetes |
| `addons/headscale_gateway/` y `yunohost/` | Paquetes generados incluidos en Git para instalación privada |

Los paquetes exportados son artefactos generados desde estas fuentes. No deben
editarse como forks. `CONTRIBUTING.md` describe el contrato de mantenimiento.

## Rutas y autenticación comunes

Las rutas de aplicación son `/admin/`, `/api/v1/…` y las rutas públicas ya
existentes del Gateway. El proxy HTTPS de servicios conserva su dominio y sus
subdominios. El panel usa el mismo registro de rutas y las mismas sesiones,
roles, MFA, tokens y CSRF en todas las instalaciones.

| Acceso | Ejemplo de panel | Ruta interna |
| --- | --- | --- |
| Directo | `http://127.0.0.1:9091/admin/` | `/admin/` |
| YunoHost | `https://equipo.example/gateway/admin/` | `/admin/` |
| Home Assistant Ingress | `/api/hassio_ingress/<token>/admin/` | `/admin/` |

El prefijo externo pertenece al anfitrión. La entrada HTTP común lo resuelve
para recursos, llamadas API, redirecciones y cookies. No hay API alternativa
para Home Assistant o YunoHost. Solo una delegación `proxy_auth` configurada
explícitamente permite identidad del proxy; exige su IP, prefijo, ID de usuario
y grupo administrativo. Home Assistant la configura para Ingress y YunoHost para
su portal. El acceso directo conserva la autenticación normal del Gateway.

## Arranque común

La configuración funcional continúa en SQLite. `GATEWAY_BOOTSTRAP_FILE` permite
proporcionar un archivo JSON privado con parámetros del proceso y valores
iniciales. Es el mismo contrato para cualquier instalación:

```json
{
  "listen_addr": "127.0.0.1:9091",
  "base_path": "",
  "base_path_header": "",
  "trusted_proxies": [],
  "allow_frame": false,
  "node_name": "gateway",
  "lan_listen_addr": "0.0.0.0:8443"
}
```

`listen_addr` es la conexión administrativa del proceso. Solo se permite HTTP
fuera de loopback cuando se configura explícitamente una lista de IP de proxies
confiables. `base_path` fija un prefijo; `base_path_header` permite recibirlo de
un proxy confiable. `allow_frame` permite insertar el panel en su mismo origen.
Ninguno de estos campos contiene nombres de plataformas.

`node_name` y `lan_listen_addr` son valores iniciales. Una actualización no
reemplaza los ajustes del usuario. La identidad VPN queda fijada en SQLite y se
adopta un único directorio de estado anterior al migrar. Se rechaza una selección
ambigua cuando existen varias identidades.

Para preparar una instalación con las mismas herramientas que los paquetes:

```bash
python3 tools/configure.py --data-dir /ruta/datos --config-dir /ruta/privada --admin admin
```

La herramienta solicita la contraseña sin ponerla en argumentos ni registros y
muestra el comando de arranque. Crea `startup.json` y un archivo privado de
credenciales iniciales. El servidor consume esas credenciales una sola vez;
no restablece contraseñas ni MFA al actualizar. Los archivos privados requieren
permisos restrictivos. Sin archivo de arranque se usan los valores configurados
en SQLite y el procedimiento interactivo habitual de primer acceso.

Todos los instaladores usan el mismo formato de estado: una carpeta del Gateway
con SQLite, `master.key`, certificados, identidad tsnet y copias. El instalador
elige su ubicación según el sistema. Cuando hay un volumen o carpeta reservada
al anfitrión, se utiliza una subcarpeta `gateway/` para permitir restauraciones
sin sustituir el punto de montaje ni las opciones del anfitrión.

## Generar las distribuciones

```bash
python3 tools/package_platforms.py
```

La versión se lee de `VERSION`. El generador escribe en
`releases/platforms-<VERSION>/`, con las carpetas `homeassistant/`, `yunohost/`
y `hacs/`, sus ZIP y `SHA256SUMS`. Cada paquete identifica sus fuentes y binarios
en `build-manifest.json`. El generador no sobrescribe una entrega existente.

El repositorio principal contiene las fuentes de la integración, pero también
aplicaciones OS: Marketplace no admite esta combinación como repositorio de
instalación. Para una posible publicación de la integración utiliza únicamente
la exportación `hacs/` como raíz del repositorio de distribución. Los
formatos de tienda de Home Assistant y de instalación de YunoHost se exportan
con sus archivos y payload: no añadas las plantillas sin generar a sus tiendas.
Los repositorios de distribución que exijan esas plataformas se generan del
mismo código; no constituyen ramas funcionales distintas.

## Home Assistant OS

El complemento usa el servidor común. Su lanzador consulta a Supervisor el
puerto de Ingress y lo traduce a `listen_addr`, configura su IP como proxy
confiable y utiliza `X-Ingress-Path` para el prefijo. No abre un segundo panel
HTTP de recuperación. El token de Supervisor se retira del entorno antes de
ejecutar el Gateway.

Ingress abre el panel compartido con tu sesión de administrador de Home Assistant,
sin otra contraseña. Requiere ID y grupo `system-admin` enviados por Supervisor;
no se emite una cookie reutilizable fuera de Ingress. Las credenciales iniciales
son opcionales para el acceso directo. Los datos
están en `/data/gateway/`; `/data/options.json` pertenece a Supervisor. Las
copias nativas son en frío. Se usa red del host para LAN y multicast, sin TUN,
NET_ADMIN ni acceso al daemon Docker.

Para publicar Home Assistant, utiliza un backend realmente alcanzable,
configura sus proxies de confianza y comprueba `/api/websocket`. La CA privada
no se instala automáticamente en Core ni en los navegadores. Más instrucciones:
`packaging/homeassistant/DOCS.md`.

## Marketplace y HACS

Desde Home Assistant 2026.11, Marketplace forma parte de Home Assistant, en
**Ajustes → Marketplace**, y sustituye a HACS. Las integraciones que instala
siguen siendo comunitarias. Marketplace no instala el servidor Gateway:
su complemento pertenece a **Ajustes → Aplicaciones**.

Marketplace y HACS requieren repositorios públicos de GitHub. Para este
repositorio privado copia `custom_components/headscale_gateway/` según
`INSTALACION_PRIVADA.md` y añade Headscale Gateway en **Dispositivos y servicios**.
No añadas el repositorio principal a Marketplace: contiene aplicaciones OS,
que Marketplace rechaza. La carpeta exportada `hacs/` sirve como base para una
publicación separada de la integración, generada desde el mismo código y
pendiente de validación del catálogo. Publicarla no es necesario para usarla
de forma privada.

La integración consulta la API común con un token delegado `viewer` y alcance
`read`. Expone siete entidades: VPN, HTTPS LAN, configuración pendiente,
servicios activos, tiempo activo, origen TLS y dominio. Admite reconfiguración,
renovación de credenciales y descarga. Una caída hace que las entidades dejen
de estar disponibles.

Usa una URL administrativa HTTPS alcanzable desde Core; la URL de Ingress del
navegador no es un endpoint de autenticación para otro proceso. No se siguen
redirecciones con el token y TLS se verifica por defecto. Consulta
`packaging/hacs/README.md`. La inclusión en el catálogo es una publicación
distinta de poder instalar un repositorio personalizado público.

Documentación oficial consultada el 4 de noviembre de 2026:
[Marketplace](https://contributors.home-assistant.io/marketplace/how-it-works/)
y [estructura admitida](https://contributors.home-assistant.io/marketplace/reference/repository-structure/).

## YunoHost

El paquete usa formato 2 y helpers 2.1, para YunoHost 12.1.17 o posterior, con
instalación, actualización, retirada, cambio de URL, backup y restore. nginx
termina HTTPS y conecta al mismo servidor administrativo por loopback. SSOwat
controla el acceso exterior y envía la identidad verificada del portal. El mismo
manejador común entra automáticamente si el usuario pertenece al grupo real
`admins`, consultado mediante NSS en cada petición. Requiere SSOwat 12.1.1+ y
una prueba privada entre nginx y el Gateway; no basta una cabecera desde loopback.
Se conserva CSRF, sin crear cookies reutilizables fuera del portal. Las cuentas
locales existentes se conservan para acceso directo.

No se solicita otra contraseña al instalar. El listener de servicios
comienza en otro puerto loopback para conservar el 443 de nginx; el 443 virtual
tsnet sigue disponible. La identidad y el dominio privado no cambian al cambiar
la URL web de YunoHost.

El estado está en `$data_dir/gateway/`. Los backups toman una instantánea con el
servicio detenido y conservan SQLite, claves e identidad juntos, también antes
de actualizar. La retirada conserva datos salvo que se solicite purgarlos.
Las copias nativas también incluyen la prueba privada de nginx y su configuración.
Consulta `packaging/yunohost/doc/ADMIN.md`.

## Verificación

```bash
./tools/verify.sh
python3 tools/check_console.py
python3 tools/check_deployment.py
CGO_ENABLED=1 go -C src test -race ./internal/admin .
```

Pruebas de la integración sobre Home Assistant Core:

```bash
python3.14 -m venv /tmp/hgw-ha-tests
/tmp/hgw-ha-tests/bin/pip install -r tests/homeassistant/requirements-2026.11.txt
/tmp/hgw-ha-tests/bin/pip check
/tmp/hgw-ha-tests/bin/pytest -q -p no:cacheprovider -c tests/homeassistant/pytest.ini tests/homeassistant
```

La versión 2026.11.0 utiliza las utilidades oficiales `pytest-homeassistant`
del mismo lanzamiento, fijadas por commit. No se mezclan con
`pytest-homeassistant-custom-component`. Para comprobar regresiones en
2026.10.4, crea otro entorno y utiliza `requirements-2026.10.txt`.
La integración usa las mismas fuentes en ambas versiones.
Las pruebas incluyen configuración, siete entidades, reconfiguración,
renovación de credenciales y un servidor HTTP local que comprueba peticiones
Bearer, caída y recuperación, y reautenticación automática tras un 401.
No necesitan acceder a Forgejo ni a una instalación real del usuario.

Resultado del 4 de noviembre de 2026: **12 pruebas correctas en Core 2026.10.4
y 12 en Core 2026.11.0**, con dependencias coherentes en entornos separados.
Se ejecuta Core real; la API usada por la prueba HTTP es local y controlada.
La comprobación de Ingress, VPN y restauración en instalaciones reales de OS
continúa siendo independiente de estas pruebas de integración.

Prueba de la imagen con datos desechables y una IP de Supervisor simulada:

```bash
docker build --build-arg BUILD_VERSION="$(cat VERSION)" --build-arg BUILD_ARCH=amd64 \
  -t hgw-ha addons/headscale_gateway
docker network create --subnet 172.30.32.0/24 hgw-test
docker run --rm --network hgw-test --ip 172.30.32.2 \
  --add-host supervisor:127.0.0.1 --entrypoint python3 \
  -v "$PWD/tools/check_container.py:/check_container.py:ro" \
  -v "$PWD/addons/headscale_gateway/config.yaml:/addon-config.yaml:ro" hgw-ha /check_container.py
docker network rm hgw-test
```

SSO de YunoHost con nginx y NSS reales y una identidad de portal simulada:

```bash
docker build -f tests/containers/yunohost-sso.Dockerfile -t hgw-ynh-sso yunohost
docker run --rm -v "$PWD/tools/check_yunohost_sso.py:/check_yunohost_sso.py:ro" hgw-ynh-sso
```

La prueba usa datos desechables y comprueba raíz/subruta, identidad administrativa,
CSRF, retirada del grupo, falsificación de cabeceras y persistencia de la prueba
privada al cambiar la URL. No sustituye una prueba completa de SSOwat/YunoHost.

Las pruebas comprueban las mismas rutas en las tres formas de acceso,
inicialización, persistencia de credenciales e identidad, CSRF, restauración,
cierre e integridad SQLite. La aceptación en Home Assistant OS con Supervisor
real y `package_check` en una VM YunoHost sigue requiriendo esas instalaciones:
compilar y simular un proxy no demuestra el funcionamiento completo de la red
objetivo, sus ACL, DNS, certificados o hardware ARM64.
