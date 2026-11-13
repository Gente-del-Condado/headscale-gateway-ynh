# Antes de instalar

Necesitas un servidor Headscale existente y un dominio DNS para tus servicios
privados.

El paquete admite amd64 y arm64 en YunoHost 12.1.17 o posterior. Selecciona
un dominio HTTPS y una ruta para el panel administrativo; pueden ser distintos
del dominio privado de los servicios. No sustituye al servidor Headscale.

El panel se abre desde el portal de YunoHost con una cuenta del grupo `admins`,
sin otra contraseña del Gateway. Se requiere SSOwat 12.1.1 o posterior y que el
sistema pueda consultar ese grupo mediante NSS. El instalador comprueba ambos
requisitos.

El código propio es software libre bajo AGPL-3.0-only. El paquete incluye sus
fuentes completos, dependencias y avisos de terceros.

La incorporación al catálogo oficial requiere revisión y pruebas de YunoHost.
Si vas a actualizar una instancia,
conserva la copia de seguridad de YunoHost y los datos del Gateway.
