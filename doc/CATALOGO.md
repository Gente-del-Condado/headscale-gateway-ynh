# Preparar la solicitud al catálogo oficial

Identificador: `headscale_gateway`. Repositorio de paquete:
https://github.com/Gente-del-Condado/headscale-gateway_ynh (rama `main`).
Licencia del código propio: AGPL-3.0-only; los avisos y licencias de terceros se
conservan. El paquete incluye los fuentes correspondientes del servidor en
`payload/source.tar.gz`, sus instrucciones, sumas y avisos.

## Qué se entrega

Gateway de servicios privados para un Headscale existente: nodo tsnet, proxy
HTTPS, DNS, certificados, administración, perfiles y copias cifradas.
Complementa al paquete `headscale_ynh`; no instala otro servidor de coordinación
ni exige sustituir el que ya utiliza el administrador.

El paquete usa formato 2, helpers 2.1, puertos asignados por YunoHost, usuario y
directorios propios, permisos SSO iniciales para administradores, nginx y
servicio systemd. Admite amd64/arm64, subruta, raíz y varias instancias con datos
separados. Incluye operaciones de actualización, copia, restauración y cambio
de URL; deben pasar las pruebas de integración antes de declararlas aceptadas.
La actualización conserva las claves, la identidad VPN y los datos persistentes.

La aceptación aislada detectó y corrigió el nombre inicial inválido de la
segunda instancia. Los sufijos de instancia se convierten en un nombre de nodo
normalizado antes de arrancar el motor. Las pruebas SSO comprueban también la
API protegida; recibir el HTML del panel no acredita una sesión. La auditoría
adicional verifica el rol administrador y el rechazo de cabeceras falsificadas
sin guardar el token CSRF de la respuesta.

También se repone el modo privado y el propietario de `startup.json` después
de restaurar el directorio instalado: el motor rechaza un archivo con permisos
abiertos. La reparación conserva el contenido y rechaza enlaces simbólicos.
La ejecución aislada del 13 de noviembre de 2026 pasó las siete comprobaciones
en amd64 con YunoHost 12.1.41.2: linter, raíz, subruta, multiinstancia,
copia/restauración, actualización desde la misma versión y cambio de URL.
No se atribuyen a esta ejecución pruebas en arm64 o desde versiones anteriores.

## Evidencias y solicitud

1. Regenerar con `python3 tools/prepare_git_install.py` y ejecutar la verificación
   del proyecto y el linter oficial contra `yunohost/`.
2. Ejecutar `package_check` oficial en una VM desechable. Guardar resultados de
   instalación raíz/subruta, permisos, reinstalación, multiinstancia, actualización,
   copia/restauración y cambio de URL. La instalación personal no los sustituye.
3. Comprobar acceso público a los fuentes, al manual, a la licencia y a las URL
   del paquete. Revisar las dependencias con los controles de seguridad.
4. Preparar un fork de `YunoHost/apps`, añadir la entrada alfabética y
   `logos/headscale_gateway.png`, usando el logo existente. No asignar `level`:
   lo calcula la infraestructura oficial. Categoría `system_tools`, subtag
   `network`.
5. Adjuntar las limitaciones y los registros reales al cuerpo de la solicitud.
   La incorporación al catálogo y la calificación solo corresponden a YunoHost.

Los avisos del linter por no estar aún en el catálogo, no tener categoría en él
o no figurar como `working` son esperables antes de la incorporación; no prueban
un defecto del instalador ni deben ocultarse simulando una entrada oficial.
En esta ejecución generan un nivel global local `0 (Broken)`, aunque las siete
comprobaciones figuren correctas: el crítico es
`AppCatalog.is_in_catalog` y el error es `AppCatalog.state_is_working`.
No se presenta ese nivel como una calificación oficial de la aplicación.

El material revisable de esta entrega se genera en
`builds/production-1.4.7/yunohost-catalog/`. No se envía una solicitud upstream
automáticamente y no se instala nada en el YunoHost personal durante estas
comprobaciones.

Referencias oficiales: [política](https://doc.yunohost.org/en/dev/packaging/policy/),
[pruebas](https://doc.yunohost.org/en/dev/packaging/test/) y
[publicación](https://doc.yunohost.org/en/dev/packaging/publish/).
