# Instalación completada

Abre **Headscale Gateway** desde el portal de YunoHost con tu cuenta
administradora. Entrarás directamente, sin otro inicio de sesión del Gateway.

Configura la URL de Headscale y el dominio base de servicios en el panel;
después reinicia desde el propio Gateway. El dominio HTTPS público de YunoHost
y el dominio privado del Gateway son ajustes independientes.

El panel administrativo está en la subruta `/admin/` de la aplicación.
Para las próximas actualizaciones, utiliza el clon privado y el comando
`yunohost app upgrade` con la carpeta `yunohost/` del repositorio.

Comprueba el estado de la VPN antes de añadir tu primer servicio. Utiliza un
backend que el servidor pueda alcanzar y prueba su URL publicada desde un
cliente autorizado. Si utilizas la CA local, instala su certificado público
en ese cliente. El certificado HTTPS del portal no sustituye esa confianza.

Crea una copia tras completar la configuración. No borres SQLite, `master.key`,
certificados ni identidad VPN para actualizar. Evita arrancar una restauración
con la misma identidad mientras el nodo original continúa conectado.

La guía incluida en el paquete, `README.md`, explica actualización, red,
recuperación y diagnóstico. Si no abre el panel, revisa la pertenencia al grupo
`admins` y el estado del servicio de esta instancia.
