# Solución de problemas

Empieza por identificar dónde falla el recorrido. El navegador llega al panel o al listener de servicios, el Gateway resuelve el backend y la red permite la conexión. Cada tramo tiene sus propios permisos y certificados.

## El panel no abre

| Forma de instalación | Primera comprobación |
| --- | --- |
| Directa | Proceso en marcha, ruta `--db` correcta y `http://127.0.0.1:9091/admin/` desde el mismo equipo. |
| Home Assistant | Cuenta administradora, complemento arrancado y acceso desde **Abrir interfaz web**. |
| YunoHost | Cuenta del grupo `admins`, enlace de la aplicación y servicio de la instancia activo. |
| Android | Motor iniciado, permisos de VPN y estado de la aplicación. |

En Home Assistant, actualiza la carpeta del complemento y reconstruye cuando cambie su contenido. En YunoHost, consulta el servicio de la instancia con `systemctl status headscale_gateway` y sus mensajes con `journalctl -u headscale_gateway -n 100 --no-pager`. Para una segunda instancia utiliza su nombre real, por ejemplo `headscale_gateway__2`.

## El panel responde 401 o 403

Un 401 señala que la autenticación no se ha aceptado. Comprueba sesión, caducidad o token. Un 403 puede corresponder al rol, los alcances, la política Bearer, CSRF o un requisito de sesión interactiva.

Con Ingress, verifica que Home Assistant transmite el identificador y grupo de administrador. Con YunoHost, revisa la pertenencia real a `admins`. Utiliza los mecanismos de acceso de cada anfitrión antes de recurrir al procedimiento de [recuperación](RECUPERAR_ACCESO.md).

## El nombre resuelve y el servicio no conecta

1. Confirma que la dirección corresponde a LAN o VPN según el cliente.
2. Comprueba el puerto. El 443 de tsnet y el puerto LAN, inicialmente 8443, son listeners diferentes.
3. Revisa ACL, rutas aprobadas y cortafuegos.
4. Comprueba el backend desde el anfitrión del Gateway.
5. Consulta los mensajes del canal proxy.

Si el backend es HTTPS, su certificado también debe ser válido para el nombre que usa el Gateway. La confianza hacia el backend y la confianza del navegador hacia el Gateway se verifican en tramos distintos.

## El certificado no es de confianza

Comprueba fecha del equipo, nombre o IP visitados y autoridad emisora. Con CA local, instala el certificado público tras comprobar la huella. El nombre debe figurar en el certificado. El puerto se utiliza para conectar y queda fuera de los nombres SAN.

La descarga de una CA y su aprobación son pasos explícitos. La confianza en un navegador tampoco configura automáticamente Home Assistant Core ni todos los clientes del sistema.

## Se ha guardado un ajuste y sigue el comportamiento anterior

Revisa el aviso de reinicio pendiente. Las opciones del proceso necesitan reinicio, mientras que los servicios y algunas opciones del portal se recargan. Consulta **Acerca de** para conocer la versión realmente ejecutada. En Home Assistant, copiar los archivos de instalación debe ir seguido de actualización o reconstrucción del complemento.

## SQLite está ocupada o la identidad se duplica

Comprueba que solo hay un proceso utilizando esa ruta de datos. Conserva juntos `gateway.sqlite`, `master.key` y el estado necesario para recuperar la instalación. Para otro nodo, crea una instalación con datos e identidad nuevos. Para migrar una identidad, apaga la instancia original antes de iniciar la restaurada.

## Una copia no se puede restaurar

Verifica la contraseña, el archivo completo y sus componentes. Una restauración completa detiene el motor y sustituye datos locales. Realiza primero la prueba en un entorno aislado. La [guía de uso](USO_COMPLETO.md#copias-actualización-y-migración) explica qué datos y operaciones remotas están incluidos.

## Cómo comunicar un fallo

Anota la versión, plataforma, hora del fallo, pasos para reproducirlo y resultado esperado. Adjunta el identificador del evento y un fragmento de registro revisado. Los ejemplos deben usar nombres ficticios y excluir contraseñas, claves privadas, tokens y copias de datos.

Para desarrollo, reproduce el caso con las pruebas de [PRUEBAS.md](PRUEBAS.md) y registra qué comprobaciones se han ejecutado. Un resultado histórico de otra versión solo sirve de referencia.
