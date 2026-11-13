# Privacidad de Headscale Gateway para Android

Actualizado el 13 de noviembre de 2026 para la versión 1.4.7.

Headscale Gateway ejecuta un nodo de tu red Headscale y publica los servicios
que configuras. El desarrollador no proporciona una cuenta, un servidor VPN
central ni un servicio de recopilación de datos. No se incluyen publicidad,
analítica de uso ni SDK de seguimiento.

## Datos y conexiones

La configuración, las credenciales, las claves VPN, los certificados y la base
SQLite se guardan en el almacenamiento privado de la aplicación. Android
Keystore protege los datos de acceso iniciales. Las copias automáticas del
sistema están deshabilitadas. Los registros de diagnóstico permanecen en el
dispositivo salvo que tú decidas compartirlos; revísalos antes de hacerlo.

La aplicación se comunica con el servidor Headscale que indiques, con los nodos
de tu red y con los servicios DNS, ACME, identidad o repositorios que actives.
Estos operadores pueden recibir direcciones IP, nombres de nodo, solicitudes
DNS y otros metadatos necesarios para la función configurada. Sus políticas y
su conservación de datos dependen de sus respectivos administradores.
No configures servidores que no sean de confianza.

## Permiso VPN

**Conectar aplicaciones** solicita tu consentimiento antes del permiso VPN de
Android. El adaptador procesa tráfico TCP, UDP y DNS de las aplicaciones del
perfil. El tráfico de la red privada se cifra entre los nodos mediante el motor
Tailscale. Sin nodo de salida, Internet usa la conexión habitual del teléfono a
través del adaptador; con un nodo de salida configurado, pasa por ese nodo.
El administrador del nodo de salida puede observar los metadatos de conexión
y el tráfico que no esté protegido además por HTTPS u otro cifrado de extremo
a extremo. La aplicación no vende ni redirige tráfico con fines publicitarios.

Android solo permite una VPN activa por perfil. Puedes desconectarla desde la
aplicación o los ajustes del sistema. Desconectar las aplicaciones conserva el
Gateway; **Detener todo** detiene también el nodo y sus servicios. No se ofrece
VPN siempre activa ni bloqueo de conexiones al iniciar el teléfono.

## Archivos, exportaciones y eliminación

El selector de Android permite elegir dónde guardar o leer una exportación.
No se solicita acceso general al almacenamiento. Los perfiles y las copias
cifradas solo se suben a un repositorio cuando configuras y solicitas esa
función. Conserva su contraseña de recuperación fuera de la aplicación.

Puedes borrar los datos locales desde los ajustes de Android o desinstalar la
aplicación. Esto no elimina las copias que hayas exportado ni el registro del
nodo en Headscale: elimínalos en sus destinos si también quieres retirarlos.

## Contacto y cambios

Para consultas de privacidad utiliza las
[incidencias del proyecto](https://github.com/Gente-del-Condado/headscale-gateway_ynh/issues).
No publiques contraseñas, claves, copias ni información de tu red en una
incidencia pública. Los cambios de tratamiento se documentarán aquí y, cuando
corresponda, requerirán un nuevo consentimiento en la aplicación.
