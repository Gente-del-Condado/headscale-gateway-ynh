# Puertos LAN y descarga inicial de CA

La CA pública puede obtenerse sin HTTPS para establecer la confianza inicial. HTTP no autentica al emisor: **descargar no equivale a confiar**. Las claves privadas nunca se publican. Las CA recibidas por HTTP se guardan pendientes y su huella SHA-256 debe contrastarse por otro canal antes de aprobarlas.

## Puertos configurables

En **Sistema → Configuración → Comunicaciones LAN**:

| Ajuste | Valor inicial | Uso |
| --- | --- | --- |
| Escucha HTTPS LAN (`server.listen_addr`) | `0.0.0.0:8443` | Portal, servicios y administración HTTPS si está habilitada |
| Publicar CA por HTTP (`pki.http_enabled`) | Desactivado | Activa el listener aislado de CA pública |
| Dirección y puerto HTTP de CA (`pki.http_listen_addr`) | `0.0.0.0:9080` | Solo `/pki/v1/public-ca` |
| Permitir descarga HTTP (`pki.allow_http_download`) | Desactivado | Autoriza al receptor a usar HTTP como último recurso |
| Puerto UDP de descubrimiento (`pki.discovery_port`) | `47991` | Todos los gateways que se busquen en LAN deben compartirlo |

En el grupo Administración puedes cambiar `admin.listen_addr`, manteniendo siempre una dirección loopback, por ejemplo `127.0.0.1:19091`. Ese HTTP no se expone en LAN. Tailscale conserva su listener HTTPS 443; los puertos LAN son independientes.

Guarda y reinicia después de modificar estos ajustes. Los puertos TCP/UDP deben estar permitidos por el cortafuegos correspondiente; el Gateway no abre reglas automáticamente. Para evitar privilegios especiales, normalmente resulta cómodo escoger puertos superiores a 1024.

Si el puerto de CA HTTP está ocupado, el Gateway informa del error y conserva administración. Si falla el puerto HTTPS LAN y administración está habilitada, el panel local también permanece disponible para cambiarlo; la VPN puede continuar funcionando. Si el propio puerto administrativo está ocupado, hay que resolver ese conflicto antes de arrancar. No ejecutes dos procesos contra la misma SQLite.

## Ejemplo entre Windows y Linux

En el Gateway que comparte la CA:

1. Elige, por ejemplo, HTTPS LAN `0.0.0.0:19443` y HTTP de CA `0.0.0.0:19080` si los puertos habituales están ocupados.
2. Activa **Publicar CA por HTTP en LAN**, guarda y reinicia.
3. En **CA y confianza**, activa **Publicar certificado público**. Ambos permisos son necesarios. Desactivar la publicación hace que el endpoint deje de entregar la CA aunque el listener siga abierto.
4. Desde un equipo LAN, comprueba `http://IP_LAN_DEL_GATEWAY:19080/pki/v1/public-ca`. Debe entregar únicamente un certificado PEM. `/admin/` y `/api/v1/status` en ese puerto deben devolver 404.

En el Gateway receptor:

1. Activa **Permitir descarga de CA por HTTP LAN** y reinicia.
2. Comprueba que ambos usan el mismo puerto UDP de descubrimiento y que la LAN permite multicast.
3. En **CA y confianza**, pulsa **Descubrir y descargar CA pendientes**.
4. El orden es Tailscale HTTPS 443, HTTPS LAN en el puerto anunciado y, al final, HTTP LAN en el puerto de CA anunciado.
5. Compara la huella con el administrador del emisor por un canal independiente. Solo después pulsa aprobar.
6. La federación de catálogos y servicios continúa usando **HTTPS verificado**, con el puerto HTTPS del Gateway. No usa el puerto HTTP de CA ni envía allí tokens o cookies.

Si multicast no llega entre las redes, puedes descargar el certificado público desde la URL HTTP conocida e importarlo manualmente como pendiente, indicando la URL **HTTPS del portal principal** del emisor. No uses la URL HTTP como endpoint del catálogo.

## Límites del HTTP público

- Solo GET/HEAD de la ruta exacta `/pki/v1/public-ca`; sin administración, proxy, claves privadas ni credenciales.
- El listener acepta orígenes LAN privados/loopback. El receptor fija la IP privada antes de conectar, limita tamaño/tiempo y rechaza redirecciones y destinos públicos.
- HTTP está desactivado de fábrica tanto para publicar como para recibir. No se utiliza para catálogos, sincronización de CA aprobadas ni para degradar conexiones Tailnet.
- Un anuncio LAN puede falsificarse. La huella mostrada en la propia respuesta HTTP no es una comprobación independiente: compárala con la que ves en el equipo emisor o por otro canal autenticado.
- El número de puerto no forma parte de los SAN del certificado.

## Mini GUI compacta

La vista habitual mide 340 × 360 píxeles lógicos. Es una ventana flotante sin marco, con tema claro/oscuro del sistema, situada junto a la bandeja cuando el escritorio permite posicionarla. Equipos está plegado inicialmente; preferencias tiene su propia vista, con desplazamiento y altura máxima de 560.

Clic en la bandeja abre/oculta. Clic fuera de la vista habitual o Escape la oculta. Mientras editas preferencias no se cierra al perder foco. La X oculta; **Salir** termina el proceso. La sesión/token recordado y las notificaciones mantienen su funcionamiento.

En Wayland, el compositor puede decidir la posición e impedir colocar exactamente la ventana junto al reloj. El panel sigue funcionando y puede moverse usando su cabecera. No se ha convertido Electron en una aplicación nativa del toolkit del escritorio: se ha adaptado su comportamiento y tamaño.
