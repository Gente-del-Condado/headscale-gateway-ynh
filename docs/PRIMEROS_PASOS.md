# Primeros pasos

El objetivo de este recorrido es abrir el panel, conectar un Gateway a Headscale y publicar un servicio de prueba. Utiliza un nodo y un servicio que puedas probar antes de aplicarlo a tu instalación habitual.

## Reúne los datos de la instalación

| Dato | Ejemplo ficticio | Dónde se utiliza |
| --- | --- | --- |
| URL de Headscale | `https://headscale.example.net` | Conexión del nodo VPN y API de Headscale |
| Nombre del nodo | `casa` | Identidad y nombres de los servicios |
| Dominio de servicios | `intra.example.net` | Portal, servicios y administración HTTPS |
| Destino de prueba | `http://192.0.2.20:8080` | Dirección que debe alcanzar el proceso Gateway |
| Certificados | CA local o ACME DNS-01 | HTTPS de los servicios |
| DNS | Manual, Technitium o Cloudflare | Resolver nombres hacia el Gateway |

Las direcciones `example.net` y `192.0.2.0/24` son ejemplos. Sustitúyelas por direcciones alcanzables en tu instalación. La clave de API de Headscale permite consultar y administrar su API. La clave de alta de nodos autoriza la incorporación a la VPN. Comprueba qué credencial solicita cada campo del panel.

## Elige dónde ejecutar el Gateway

- **Home Assistant OS**. Sigue [la guía del complemento](../packaging/homeassistant/DOCS.md). Abre su interfaz con una sesión administradora de Home Assistant.
- **YunoHost**. Sigue [la guía de YunoHost](../packaging/yunohost/doc/ADMIN.md). Accede desde el portal con un usuario del grupo `admins`.
- **Android**. Consulta [instalación y uso de la APK](../android/README.md), incluidos los permisos de VPN y el estado de validación.
- **Linux, Windows o macOS**. Ejecuta el servidor con una ubicación estable para `gateway.sqlite`. El [manual de uso](USO_COMPLETO.md#instalación-y-primer-acceso) contiene comandos y directorios.

El complemento de Home Assistant ejecuta el motor. La integración de entidades se instala por separado y consulta un motor que ya está funcionando.

## Abre el panel por primera vez

En Home Assistant utiliza **Abrir interfaz web**. En YunoHost utiliza el enlace de la aplicación en el portal. En una instalación directa, abre `http://127.0.0.1:9091/admin/` desde el equipo donde ejecutas el proceso y sigue el acceso inicial de la consola.

Si necesitas entrar desde otro equipo a una instalación directa, puedes usar un túnel SSH hacia el listener local.

```bash
ssh -L 9091:127.0.0.1:9091 usuario@servidor
```

Con el túnel abierto, visita `http://127.0.0.1:9091/admin/` en tu ordenador. La URL loopback siempre se refiere al equipo desde el que se abre el navegador.

## Configura y conecta

1. En **Sistema → Configuración**, escribe la URL de Headscale y el dominio base de servicios.
2. Define el nombre estable del nodo. Conserva ese nombre después del alta.
3. Selecciona el proveedor DNS y el modo de certificados. Con DNS manual tendrás que crear los registros correspondientes.
4. Guarda. Si aparece un aviso de reinicio pendiente, reinicia desde el panel.
5. Completa la autorización de Headscale que solicite el Gateway.
6. En **Headscale → Red y nodos**, comprueba que el nodo esté conectado. Confirma sus IP y las rutas permitidas.
7. En **Sistema → Acceso / Identidad**, revisa los métodos de acceso y los roles que vas a utilizar.

El listener administrativo y los listeners de servicios tienen funciones diferentes. El acceso por Ingress o SSO permite configurar el motor antes de que su VPN esté conectada.

## Publica un servicio de prueba

1. Comprueba desde el anfitrión del Gateway que el destino responde.
2. Abre **Gateway → Servicios** y pulsa **Nuevo servicio**.
3. Utiliza `documentos` como nombre y la URL real del servicio como backend.
4. Guarda y comprueba el resultado de la recarga.
5. Revisa que `documentos.casa.intra.example.net` resuelva a la IP del Gateway que usarán los clientes.
6. Si empleas CA local, descarga su certificado público desde **Sistema → CA y confianza**, verifica la huella e instala la confianza en el cliente.
7. Abre la URL HTTPS del servicio y comprueba que el certificado corresponde al nombre visitado.

Los [ejemplos](EJEMPLOS.md) desarrollan este recorrido y el caso de Home Assistant con WebSocket.

## Prepara la recuperación

Antes de depender del Gateway, crea una copia cifrada desde **Sistema → Copias de seguridad** y guárdala fuera del equipo. Conserva su contraseña en un lugar accesible para quien deba restaurarla. Ensaya la recuperación en una instalación aislada.

Consulta [recuperar el acceso](RECUPERAR_ACCESO.md) para el código de consola y [solución de problemas](SOLUCION_PROBLEMAS.md) para los fallos habituales.
