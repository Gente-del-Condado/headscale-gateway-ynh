# Instalación y uso en Home Assistant

## Qué vas a instalar

El **complemento** ejecuta el servidor Gateway y abre su panel mediante Ingress.
La **integración de entidades** es opcional: consulta un Gateway ya instalado
y muestra su estado en Home Assistant. Se instalan por separado.

Necesitas una instalación de Home Assistant con Supervisor y soporte de
aplicaciones/complementos, arquitectura amd64 o aarch64 y permiso de administrador.
Para conectar la VPN necesitas un servidor Headscale existente. Prepara también
un nombre estable para este nodo, un dominio de servicios y los datos del DNS
que vayas a utilizar. El complemento se publica como **estable** en la tienda
de Home Assistant.

Recorrido recomendado: instalar → abrir el panel → configurar Headscale y DNS →
publicar un servicio de prueba → guardar una copia. No necesitas configurar
primero la integración de sensores para completar este recorrido.

## Abrir el panel desde Home Assistant

Desde la aplicación móvil o la web de Home Assistant, entra en **Ajustes →
Aplicaciones/Complementos → Headscale Gateway → Abrir interfaz web**. Activa
**Mostrar en la barra lateral** en la ficha para tener acceso directo desde el
menú. Utiliza una cuenta administradora de Home Assistant.

No necesitas un navegador instalado en el servidor. El puerto interno entre
62000 y 65500 pertenece a Ingress: no es un enlace para abrir en el navegador
ni debes redirigirlo desde el router. El acceso inicial al panel mediante Home
Assistant funciona antes de configurar la VPN, el DNS o los certificados del
Gateway.

Ingress utiliza directamente tu sesión de administrador de Home Assistant;
no solicita otra contraseña del Gateway. Home Assistant gestiona su inicio y
cierre de sesión y el segundo factor de autenticación. El Gateway comprueba en
cada petición la IP de Supervisor, el prefijo de Ingress, el identificador del
usuario y el grupo explícito `system-admin`. Se mantiene la protección CSRF y
no se crea una cookie del Gateway reutilizable fuera de esta conexión.

Supervisor debe enviar `X-Remote-User-Id` y `X-Remote-User-Groups`. Si faltan
estos datos, el usuario no es administrador o la petición no procede del proxy
confiable, se deniega el acceso. Mantén Home Assistant y Supervisor actualizados
si no se reciben las cabeceras de identidad.

## Instalar desde Forgejo privado

En la terminal de Home Assistant, configura primero una clave SSH con permiso
de lectura sobre el repositorio privado. Después clona el proyecto:

La dirección SSH del ejemplo es ficticia: sustitúyela por la URL SSH de tu
repositorio, con su usuario SSH, servidor, puerto y propietario reales.

```bash
GIT_SSH_COMMAND='ssh -i /data/.ssh/headscale_gateway_forgejo -o IdentitiesOnly=yes' \
git clone ssh://git@forgejo.example.net:2222/usuario/headscale-gateway.git /data/headscale-gateway-src &&
git -C /data/headscale-gateway-src config core.sshCommand \
  'ssh -i /data/.ssh/headscale_gateway_forgejo -o IdentitiesOnly=yes'
```

Este ejemplo utiliza la clave ya configurada en
`/data/.ssh/headscale_gateway_forgejo`. La guía `docs/INSTALACION_PRIVADA.md`
del repositorio explica cómo crearla y copiar su parte pública a un TXT para
abrirlo desde File Editor.

Copia el complemento preparado, sin compilarlo en la terminal:

```bash
mkdir -p /local_apps/headscale_gateway &&
cp -a /data/headscale-gateway-src/addons/headscale_gateway/. /local_apps/headscale_gateway/
```

Si tu terminal expone la carpeta local en `/addons`, utiliza esa ruta en lugar
de `/local_apps`. En la tienda de aplicaciones, busca actualizaciones e instala
**Headscale Gateway** desde **Aplicaciones locales**. Elige un `node_name`
estable, arranca y pulsa **Abrir interfaz web**. Configura en el panel la URL de
tu servidor Headscale y el dominio base de servicios.

La guía del repositorio `docs/INSTALACION_PRIVADA.md` explica cómo seleccionar
la clave SSH y copiar también la integración de entidades.

La carpeta instalable es `addons/headscale_gateway/`. La carpeta
`packaging/homeassistant/` contiene las fuentes del empaquetado y su
`config.yaml.in` es una plantilla: no la copies como otro complemento.

## Primera configuración del Gateway

1. Abre la interfaz web con tu cuenta administradora de Home Assistant.
2. En la configuración de conexión, introduce la URL HTTPS de Headscale y las
   credenciales solicitadas. Usa una clave de registro apropiada para este nodo.
3. Configura el dominio base y el proveedor DNS. Comprueba los valores antes
   de aplicar los cambios que necesiten reiniciar la conexión.
4. Comprueba el estado de la VPN y del listener HTTPS LAN en el panel.
5. Añade un servicio cuyo destino sea alcanzable desde el complemento. Por
   ejemplo, un servicio de laboratorio `http://192.168.1.50:8080` con el nombre
   `notas`. Sustituye la dirección por la de tu propia aplicación.
6. Abre la URL HTTPS que muestra el Gateway desde un cliente autorizado.
   Si utilizas la CA local, instala su certificado público en ese cliente.
7. Exporta una copia cifrada y guarda su contraseña fuera del dispositivo.

## Opciones del complemento

| Opción | Uso |
| --- | --- |
| `node_name` | Nombre DNS estable en minúsculas. No lo cambies en un nodo ya configurado. |
| `lan_port` | Puerto HTTPS LAN inicial; por defecto, 8443. Después se cambia desde el panel del Gateway. |
| `log_level` | Nivel opcional: `info`, `debug`, `trace`, `warn` o `error`. |
| `admin_username`, `admin_password` | Cuenta local inicial opcional para acceso directo; no hace falta para Ingress. |

Configuración inicial mínima, en la pestaña de opciones del complemento:

```yaml
admin_username: admin
node_name: homeassistant-gateway
lan_port: 8443
log_level: info
```

La URL de Headscale, el DNS y los servicios se configuran en el panel del
Gateway, no añadiendo campos arbitrarios a este YAML.

La contraseña inicial admite entre 12 y 72 bytes UTF-8. Solo se utiliza al
inicializar una cuenta sin configurar. Cambiar estas opciones posteriormente
no restablece una contraseña existente, LDAP, OIDC ni MFA. Las cuentas existentes
se conservan al actualizar. Configurar el acceso directo no modifica tu cuenta
ni tu sesión de Home Assistant.

## Red y publicación de servicios

La red del anfitrión permite alcanzar servicios de la LAN y descubrir otros
gateways mediante multidifusión. El complemento no solicita TUN, NET_ADMIN,
acceso al motor Docker ni desactivar el modo de protección. Su nodo tsnet es
independiente: no convierte automáticamente a Home Assistant en un router de
subred. Las rutas necesitan permisos ACL y aprobación en Headscale.

El puerto LAN elegido debe estar libre. El panel administrativo utiliza otro
puerto de Ingress asignado dinámicamente y solo acepta a Supervisor en
`172.30.32.2`. No publiques ese puerto directamente.

Para publicar el propio Home Assistant como servicio del Gateway, utiliza la
dirección de destino que sea alcanzable desde el complemento; con esta red,
normalmente será `http://127.0.0.1:8123`. Configura `http.use_x_forwarded_for` y
`http.trusted_proxies` en Home Assistant para la IP real de origen del Gateway,
y comprueba `/api/websocket`. Esta publicación es independiente del acceso
administrativo mediante Ingress.

Los registros de Cloudflare para servicios privados deben utilizar solo DNS,
sin su proxy HTTP. El DNS debe resolver al nodo LAN o VPN correspondiente.
Confía en la CA local desde cada cliente o configura ACME: instalar una CA en
el complemento no la instala automáticamente en Core ni en los navegadores.

## Actualizar sin perder datos

Conserva una copia de seguridad antes de actualizar. En la terminal:

```bash
git -C /data/headscale-gateway-src pull --ff-only &&
cp -a /data/headscale-gateway-src/addons/headscale_gateway/. /local_apps/headscale_gateway/
```

Después busca actualizaciones y actualiza o **reconstruye** el complemento.
Copiar archivos no reemplaza el contenedor que está ejecutándose. No hace falta
desinstalar para actualizar.

Todo el estado está en `/data/gateway/` dentro del contenedor: SQLite,
`master.key`, identidad tsnet, certificados y copias. `/data/options.json`
pertenece a Supervisor. La actualización conserva estos datos; no los copies
desde Git ni los sustituyas por los de otro nodo.

Las copias nativas son en frío, con el servicio detenido. También puedes usar
las exportaciones `.hgw` del Gateway. Una restauración completa sustituye la
subcarpeta `gateway/`, no el montaje `/data` ni las opciones de Supervisor.
No ejecutes la misma identidad restaurada simultáneamente en dos equipos.

## Recuperar el acceso

Si **Abrir interfaz web** devuelve un 404, comprueba que has reconstruido el
complemento con los archivos actuales. La entrada correcta en el manifiesto es
`ingress_entry: admin/`, relativo al prefijo que añade Supervisor.
Después de copiar los archivos actualizados, recarga la tienda de aplicaciones,
actualiza o reconstruye el complemento y vuelve a abrirlo desde su ficha.
Cierra las pestañas antiguas; si el navegador recuerda la redirección 301,
prueba desde una ventana privada o borra la caché del sitio.
El ajuste de publicación privada del Gateway no bloquea este acceso: Ingress
utiliza el listener administrativo, separado del acceso HTTPS por LAN o VPN.

Si el contenedor sigue mostrando errores de una versión antigua después de
copiar los archivos, reconstruye la imagen y comprueba su versión. Conserva
`panel_admin: true`: la opción de mostrar la barra lateral y el permiso de
administración son ajustes diferentes.

Para entrar normalmente, utiliza tu cuenta administradora de Home Assistant.
El cierre de sesión y MFA de ese acceso se gestionan en Home Assistant.

Para una recuperación de emergencia del acceso propio del Gateway, detén el
proceso y ejecuta el servidor en una terminal interactiva con la misma ruta
`--db` y `--reset-recovery`. Vuelve a iniciarlo interactivamente para obtener
el código de un solo uso. No abras la misma base de datos con dos procesos a
la vez. Los códigos no se escriben en los registros del complemento.

Otra posibilidad es restaurar una copia verificada en un entorno aislado.
El paquete conserva la licencia del proyecto y los avisos de terceros originales.
Las compilaciones normales no activan el módulo comercial opcional.

## Diagnóstico rápido

| Síntoma | Qué revisar |
| --- | --- |
| No aparece en aplicaciones locales | Ruta que expone tu terminal (`/local_apps` o `/addons`), copia completa y recarga de la tienda |
| Ingress responde 403 | Cuenta administradora, Supervisor actualizado y acceso desde la ficha del complemento |
| No arranca HTTPS LAN | Puerto ocupado y registro del complemento |
| Un servicio devuelve 502 | Dirección, puerto y protocolo del backend vistos desde el complemento |
| Certificado no confiable | CA instalada en el cliente que abre el servicio o certificado ACME válido |
| Sensores no disponibles | URL administrativa y token de la integración; Ingress no es su endpoint API |

Al pedir ayuda, incluye versión, arquitectura, forma de acceso y el error sin
credenciales. Evita compartir bases de datos, copias, opciones con contraseñas
o registros completos sin revisar.

## Licencia y fuentes

El código propio es software libre bajo **AGPL-3.0-only**. `source.tar.gz`
contiene las fuentes del motor, `LICENSE`, `NOTICE` y `SOURCE.md` con las
instrucciones de compilación. Los avisos de terceros se conservan en `notices/`.
