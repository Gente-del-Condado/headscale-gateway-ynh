# Headscale Gateway para Home Assistant

Gateway de servicios HTTPS privados sobre Headscale, con nodo Tailscale
integrado, gestión DNS, certificados CA/ACME, panel web, copias de seguridad
y federación entre gateways.

Necesita un servidor Headscale existente. Para instalarlo desde tu Forgejo
privado, copia `addons/headscale_gateway/` del clon a la carpeta de aplicaciones
locales de Home Assistant. Consulta la [guía completa](DOCS.md): requisitos,
primera configuración, opciones, actualización, copias y resolución de problemas.

Abre el panel con **Abrir interfaz web** y activa **Mostrar en la barra lateral**
para acceder desde el menú. Entrarás con tu sesión administradora de Home
Assistant, sin otra contraseña del Gateway. Las credenciales iniciales son
opcionales y sirven para configurar una cuenta separada para el acceso directo.

El complemento se publica como estable en la tienda de Home Assistant. Consulta
`DOCS.md` para conocer los requisitos de red, las copias y la recuperación.
