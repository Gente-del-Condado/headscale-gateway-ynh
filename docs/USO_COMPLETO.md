# Headscale Gateway

**Home Assistant OS, integración de entidades y YunoHost:** consulta
[los paquetes y su instalación](PLATAFORMAS.md). Una sola aplicación comparte
el mismo servidor, panel, API, autenticación y datos en todas las instalaciones.
La integración está en `custom_components/headscale_gateway/`. Desde Home Assistant
2026.11, Marketplace sustituye a HACS; el acceso desde Forgejo privado sigue
utilizando la copia local documentada.

**Forgejo privado, sin ZIP:** sigue [la instalación con git pull](INSTALACION_PRIVADA.md).
`addons/headscale_gateway/` se copia a Home Assistant; `yunohost/` se instala
desde el clon local. Ambas carpetas se generan del mismo código del repositorio.

**Android experimental:** [compilación y uso de la APK](../android/README.md), con
el mismo motor y panel, servicio Android y adaptador VPN basado en HEV.
Consulta [el estado de validación](ANDROID.md) antes de usarla como servidor permanente.

[Instalación privada](INSTALACION_PRIVADA.md) · [Recuperar acceso](RECUPERAR_ACCESO.md) · [Licencias](LICENCIAS.md) · [Suscripciones](SUSCRIPCIONES.md)

Gateway de servicios privados para redes gestionadas por **Headscale**, con VPN integrada mediante **Tailscale tsnet**, proxy inverso HTTPS, DNS, certificados, panel de administración y cliente independiente de bandeja para Windows y Linux.

La configuración se guarda en **SQLite**. El servidor no usa `config.yaml` como configuración activa. La mini GUI se conecta a la API; no sustituye ni arranca el servidor.

> Estado del proyecto: desarrollo activo. Las pruebas automatizadas se ejecutan en Linux y se compilan binarios para Windows y macOS. Los resultados históricos de otras plataformas no equivalen a una validación de cada nueva versión. La federación entre dos gateways físicos está pendiente de la prueba descrita en este manual.

## Índice

- [Qué incluye](#qué-incluye)
- [Arquitectura y direcciones](#arquitectura-y-direcciones)
- [Requisitos](#requisitos)
- [Instalación y primer acceso](#instalación-y-primer-acceso)
- [Configuración inicial](#configuración-inicial)
- [Servicios, DNS y certificados](#servicios-dns-y-certificados)
- [Panel de administración](#panel-de-administración)
- [Portal privado y federación](#portal-privado-y-federación)
- [Mini GUI y notificaciones](#mini-gui-y-notificaciones)
- [API y CLI](#api-y-cli)
- [Identidad y permisos](#identidad-y-permisos)
- [Recuperar el acceso](RECUPERAR_ACCESO.md)
- [Activación y suscripciones](SUSCRIPCIONES.md)
- [Copias, actualización y migración](#copias-actualización-y-migración)
- [Registros e idiomas](#registros-e-idiomas)
- [Compilar y verificar](#compilar-y-verificar)
- [Publicar en Forgejo o GitHub](#publicar-en-forgejo-o-github)
- [Solución de problemas](#solución-de-problemas)
- [Límites y licencia](#límites-y-licencia)

## Qué incluye

| Componente | Función |
| --- | --- |
| Gateway Go | Nodo tsnet, listeners HTTPS, proxy y ciclo de certificados |
| Panel web | Configuración, servicios, red, usuarios, ACL, CA, copias y diagnósticos |
| Portal privado | Enlaces a servicios locales y remotos seleccionados por el administrador |
| Federación | Descubrimiento opcional, aprobación de confianza y catálogo HTTPS versionado |
| Mini GUI Electron | Estado, rutas pendientes, notificaciones y acceso al panel |
| CLI | Arranque, mantenimiento, versión y diagnóstico de registros |
| Integraciones | Headscale, Technitium, Cloudflare, ACME, LDAP, OIDC y repositorios de plantillas |

No hace falta instalar el cliente Tailscale del sistema para que el Gateway cree su propio nodo tsnet. El estado y las ACL de ese nodo pueden ser diferentes de los del cliente Tailscale instalado en el mismo ordenador.

## Arquitectura y direcciones

```text
Navegador o mini GUI
        │ HTTPS privado / HTTP loopback
        ▼
Gateway ── SQLite, identidad tsnet, certificados, logs
  │  ├── API administrativa → Headscale / proveedor DNS
  │  ├── Portal → servicios locales y catálogo de peers aprobados
  │  └── Proxy HTTPS → backends configurados
  └──── Headscale / tailnet, sujeto a ACL
```

Para un nodo llamado `equipo` en la zona `intra.example.net`:

| Dirección | Uso |
| --- | --- |
| `http://127.0.0.1:9091/admin/` | Administración de recuperación, solo desde el equipo |
| `https://equipo.intra.example.net/` | Portal principal sobre tailnet |
| `https://servicio.equipo.intra.example.net/` | Servicio publicado |
| `https://admin.equipo.intra.example.net/admin/` | Administración HTTPS si está habilitada |
| `https://equipo.intra.example.net:8443/` | Listener LAN si está habilitado y DNS apunta a la LAN |
| `/.well-known/headscale-gateway` | Catálogo opcional del portal principal |
| `/pki/v1/public-ca` | Certificado CA público opcional; nunca la clave privada |

El 443 de tsnet y el puerto del listener LAN son cosas distintas. La resolución DNS debe llevar al listener elegido. No abras el HTTP de recuperación a la LAN ni a Internet.

## Requisitos

- Acceso al servidor Headscale y a una tailnet autorizada.
- Dominio base resoluble por los clientes. DNS configurado manualmente o mediante Technitium/Cloudflare.
- Certificado TLS válido: ACME DNS-01 o CA local confiada por los clientes.
- Para funciones de inventario/administración: clave REST de Headscale. Una clave de alta de nodos no la sustituye.
- Para publicación DNS automática: credenciales del proveedor y permisos sobre la zona elegida.
- Windows 11 x64 o Linux x64 para las distribuciones de bandeja preparadas. Linux necesita sesión gráfica y soporte de bandeja de su escritorio.
- Para compilar: versión de Go indicada en `src/go.mod` (actualmente 1.27.4), Python 3.11 o posterior para herramientas de validación y Node.js 22 o compatible para pruebas/cliente.

Los binarios de servidor no necesitan Node.js. La mini GUI empaquetada incluye Electron; conserva todos sus archivos juntos. macOS tiene código y compilaciones del servidor; la GUI macOS no forma parte de la entrega probada.

## Instalación y primer acceso

### Linux

Extrae el paquete del servidor en una carpeta estable. Desde la carpeta extraída:

```bash
chmod +x headscale-gateway
./headscale-gateway --db "$HOME/.local/share/headscale-gateway/gateway.sqlite"
```

### Windows 11

Extrae el paquete completo. Abre PowerShell en su carpeta:

```powershell
New-Item -ItemType Directory -Force "$env:LOCALAPPDATA\HeadscaleGateway" | Out-Null
.\headscale-gateway.exe --db "$env:LOCALAPPDATA\HeadscaleGateway\gateway.sqlite"
```

En el proyecto completo, los ejecutables x64 están en `bin/linux/headscale-gateway` y `bin/headscale-gateway.exe`; los paquetes descargables los sitúan en la raíz.

No ejecutes dos instancias contra la misma base de datos. Para un segundo Gateway de pruebas usa **datos nuevos e identidad nueva**, no una copia de la identidad de un nodo en funcionamiento.

### Directorios predeterminados

| Sistema | Directorio |
| --- | --- |
| Linux usuario | `$XDG_DATA_HOME/headscale-gateway` si XDG_DATA_HOME es absoluto; si no, `~/.local/share/headscale-gateway` |
| Linux root | `/var/lib/headscale-gateway` |
| Windows | `%LOCALAPPDATA%\HeadscaleGateway`; conserva `%ProgramData%\HeadscaleGateway` si allí existe una instalación previa |
| macOS | Directorio de configuración del usuario, normalmente `~/Library/Application Support/HeadscaleGateway` |

`--db` permite elegir explícitamente la base de datos. Mantén una ubicación estable y permisos restrictivos.

### Primer acceso

En **Home Assistant**, abre **Headscale Gateway → Abrir interfaz web** con tu
sesión administradora; puedes activar **Mostrar en la barra lateral**. En
**YunoHost**, abre la aplicación desde el portal con una cuenta del grupo
`admins`. En ambos casos entrarás sin otra contraseña del Gateway. Consulta
[Instalación privada](INSTALACION_PRIVADA.md) para los pasos completos.

Para ejecutar el servidor directamente:

1. Arranca el proceso y abre `http://127.0.0.1:9091/admin/` en ese mismo equipo.
2. Sigue el flujo de bootstrap local que muestra el arranque. Guarda la credencial de recuperación en un lugar privado.
3. Completa la configuración. Una instalación nueva puede permanecer en modo inicial hasta tener dominio y Headscale válidos.
4. Configura la autenticación que quieras utilizar y verifica el acceso antes de cerrar la sesión de recuperación.
5. Para acceso remoto habilita el subdominio de administración HTTPS o utiliza un túnel SSH. Conserva el acceso loopback para recuperación.

Los scripts de instalación en `tools/scripts/` y la unidad `packaging/linux/headscale-gateway.service` son ayudas que debes revisar para tu usuario, rutas y método de ejecución. Arrancar el binario no instala automáticamente un servicio del sistema.

### Acceso y recuperación desde la consola

Al ejecutar el servidor directamente, un bloque destacado muestra el enlace local al panel. Aparece tras activar la VPN o, si tarda, a los 20 segundos de abrir administración; en el modo de configuración inicial aparece inmediatamente. Desde Home Assistant o YunoHost, las instrucciones remiten a la interfaz del anfitrión; el puerto interno del proxy no se presenta como enlace para el navegador. Los mensajes posteriores de actividad pueden seguir apareciendo.

Para recuperar el acceso, detén el proceso y ejecuta el mismo binario con `--db` apuntando a tu base de datos habitual y `--reset-recovery`. Después arráncalo normalmente en una terminal. El bloque mostrará un código de un solo uso, válido durante 15 minutos desde su aparición. Úsalo en el acceso de recuperación del panel local y cambia después la contraseña de tu cuenta en Acceso / Identidad. La rotación no modifica por sí sola esa contraseña.

El código solo se muestra en una terminal interactiva: no se guarda en los logs ni se incluye en el enlace. Si se ejecuta como servicio o con la salida de errores redirigida, queda pendiente para el siguiente arranque interactivo. En Windows se habilita el color si la consola lo admite; `NO_COLOR` permite desactivarlo. El enlace es pulsable en terminales compatibles (algunas requieren Ctrl+clic); también puede copiarse al navegador del mismo equipo.

Consulta los comandos exactos de Linux, Windows y systemd en [Recuperar acceso](RECUPERAR_ACCESO.md).

## Configuración inicial

En **Sistema → Configuración**:

1. Define el dominio base y la URL HTTPS de Headscale.
2. Configura el proveedor DNS y la zona. Si mantienes DNS manualmente, asegúrate de resolver el portal, los servicios y la administración hacia el Gateway correcto.
3. Selecciona la estrategia de certificados y, para ACME, el proveedor DNS y correo requeridos.
4. Revisa `server.listen_addr`, el listener LAN. La administración HTTP debe conservarse en loopback.
5. Activa el inventario de Headscale y su clave REST para gestionar nodos, usuarios, claves, rutas y ACL.
6. Guarda y reinicia cuando el panel lo indique. La personalización del portal se aplica directamente.
7. Completa el login de tsnet y comprueba el estado del nodo en Headscale.

El esquema vigente se consulta en **Sistema → API / CLI → GET /api/v1/config/schema**. Solo muestra las opciones que esta versión implementa; no todos los campos aceptan modificación en caliente.

## Servicios, DNS y certificados

Los puertos LAN de HTTPS, CA pública HTTP y descubrimiento UDP se configuran en **Sistema → Configuración → Comunicaciones LAN**. HTTP de CA es opcional y no implica aprobación. Consulta [Puertos y confianza inicial por LAN](RED_Y_CA_LAN.md).

### Publicar un servicio

1. Abre **Gateway → Servicios** y crea un nombre corto, por ejemplo `wiki`.
2. Indica un destino accesible **desde el Gateway**, por ejemplo `http://127.0.0.1:8080` o `http://192.168.1.20:8080`.
3. Selecciona una plantilla proxy y revisa las opciones del servicio.
4. Guarda y comprueba DNS, certificado y respuesta en `https://wiki.equipo.intra.example.net/`.
5. En **Portal privado** decide si su enlace aparece y si puede compartirse con otros gateways.

El esquema de opciones del proxy está en `GET /api/v1/proxy/schema`. Una URL visible no concede permisos de red ni sustituye la autenticación del servicio.

### DNS

Technitium y Cloudflare permiten gestionar registros de la zona configurada. Cloudflare publica los registros de VPN sin proxy. El proveedor elegido para registros y las credenciales de ACME son configuraciones que deben revisarse por separado.

MagicDNS+ ofrece exportación y herramientas para registros adicionales; no presupongas que editar el Gateway modifica automáticamente el YAML del servidor Headscale. Consulta la previsualización/exportación y aplica los cambios por el procedimiento correspondiente al servidor.

### TLS y CA

El Gateway soporta certificados ACME y certificados con CA local. Con CA local, cada cliente debe confiar en la CA pública y el nombre o IP utilizado debe estar cubierto por el certificado. **El puerto no forma parte del SAN**: una URL IP:puerto se valida contra la IP del certificado.

En **Sistema → CA y confianza** puedes descargar certificados públicos, importar y aprobar peers, gestionar la publicación y retirar una CA. Retirar del Gateway y retirar del almacén del sistema son resultados independientes; la segunda operación puede requerir privilegios.

Existe un helper opcional de instalación/retirada de CA. Se habilita de forma explícita y depende de la elevación del sistema. Consulta [HELPER_CA.txt](HELPER_CA.txt). Las pruebas privilegiadas del helper no se consideran completadas por las pruebas del portal.

## Panel de administración

| Sección | Contenido |
| --- | --- |
| Gateway / Resumen | Estado, servicios y resumen de copias |
| Servicios y plantillas proxy | Destinos, ajustes y reutilización de configuraciones |
| Portal privado | Apariencia, visibilidad, publicación y selección de servicios remotos |
| Registros | Canales, niveles, identificadores, búsqueda y configuración de visualización |
| Headscale / Red y nodos | Estado, diagnósticos, solicitudes y aprobación/revocación de rutas |
| Usuarios y claves | Gestión permitida por la API de Headscale |
| Plantillas ACL | Biblioteca, importación, exportación y repositorios |
| Reglas ACL | Editor visual/manual, revisión y aplicación con control de versión |
| CA y confianza | Autoridades locales, peers, certificados y helper |
| Copias de seguridad | Crear, descargar, restaurar y revisar programación |
| Acceso / Identidad | Autenticación, roles, MFA y recuperación |
| API / Permisos | Tokens y habilitación de ámbitos Bearer |
| API / CLI | Índice vivo, filtros, contratos, ejemplos y probador |

Pausar tsnet conserva la identidad. La interfaz distingue conectar, pausar y reanudar. El servidor impide pausar desde una sesión que depende del propio túnel que se va a cortar; utiliza el acceso local/LAN permitido para esa acción.

## Portal privado y federación

La administración del portal se realiza exclusivamente desde el panel. Permite título, descripción, tema, color, logo y favicon PNG, nombres visibles, grupos, descripción por servicio y búsqueda.

Los módulos de **publicación**, **federación** y **descubrimiento** están desactivados inicialmente y se pueden habilitar por separado. Un servicio oculto del portal sigue existiendo en el proxy: ocultarlo no es una medida de autorización.

El descubrimiento usa anuncios LAN y candidatos de Headscale contrastados con DNS. Comprueba únicamente portales principales; no recorre todos los registros de servicios de la zona. El catálogo HTTPS es la fuente de servicios compartidos. No se reexportan servicios importados ni se aprueba confianza automáticamente.

Para incluir servicios de Windows en el portal de Linux:

1. Prepara Windows como otro Gateway con identidad y nombre propios.
2. Publica su catálogo y marca los servicios que comparte.
3. En Windows habilita la publicación de su CA pública; en Linux usa **Descubrir y descargar CA pendientes** o importa el archivo manualmente. Verifica la huella por otro medio y aprueba el peer. Su URL de catálogo es el **portal principal**; la descarga intenta primero `admin.<nodo>/pki/v1/public-ca`. Tailscale usa 443 y se prioriza frente a las IP LAN conocidas.
4. Activa federación en Linux, actualiza catálogos, selecciona servicios y guarda.
5. Comprueba la aparición y apertura de enlaces desde un cliente con ACL adecuadas.

La accesibilidad se comprueba desde el Gateway receptor. Los visitantes pueden tener ACL diferentes: ver un enlace no garantiza poder abrirlo. La confianza del cliente navegador también es independiente de la confianza entre servidores.

Consulta el procedimiento completo y la prueba de Windows en [FEDERACION_WINDOWS.txt](FEDERACION_WINDOWS.txt), y los límites del protocolo en [PORTAL_Y_FEDERACION.txt](PORTAL_Y_FEDERACION.txt).

## Mini GUI y notificaciones

La vista habitual ahora es un popup de **340 × 360**, con tema del sistema, equipos plegables y preferencias aparte. Se oculta al pulsar fuera o Escape; el compositor de Wayland puede limitar su posición junto al reloj. La X oculta y Salir termina el proceso.

La mini GUI es un cliente separado. En Linux ejecuta el lanzador de la distribución `Gateway-Tray`; en Windows ejecuta `Gateway-Tray.exe`. Conserva toda la carpeta de recursos.

1. Crea un token del Gateway con rol **viewer** y ámbitos **read** y **network**. El inventario Headscale y esos ámbitos Bearer deben estar habilitados.
2. Conecta a `http://127.0.0.1:9091` para el servidor local o a su dirección base HTTPS administrativa para uno remoto. Una instalación con subruta puede usar `https://gateway.example.net/gateway`, sin añadir `/admin/` ni `/api/v1/`.
3. Introduce el token del Gateway, no la clave de Headscale ni la del DNS.
4. Decide si quieres recordar el token y activar notificaciones.
5. Usa **Probar aviso** para comprobar la integración con el escritorio.

Cerrar con la X deja la aplicación en la bandeja. **Salir** termina la aplicación. **Desconectar y olvidar token** retira la credencial guardada. El almacenamiento persistente depende del almacén seguro del sistema; si no está disponible, utiliza el token solo en memoria.

La API debe aceptar directamente el token Bearer. Una sesión del portal SSO de
YunoHost no se transfiere al cliente de bandeja: si el portal intercepta la ruta,
utiliza el panel web o una dirección administrativa accesible mediante la VPN.
No desactives SSO para resolverlo. El cliente rechaza redirecciones, HTTP remoto
y direcciones con credenciales, consulta o fragmento; limita las respuestas a
4 MiB y cada petición a diez segundos.

Notifica solicitudes de rutas, caída y recuperación sin repetir continuamente el mismo evento. Silenciar no detiene la consulta de estado. Las aprobaciones se hacen en el panel administrativo. No hay arranque automático del sistema configurado por defecto.

## API y CLI

### Documentación integrada

**Sistema → API / CLI** obtiene las rutas del mismo registro que utiliza el servidor, incluidas las rutas generadas por módulos. Filtra por texto, método y módulo; muestra ámbito, roles, tipo de autenticación, archivo fuente y contrato del controlador.

- **API Gateway:** operaciones REST de esta versión.
- **CLI:** subcomandos y opciones reales, con el archivo donde se definen.
- **Integraciones externas:** operaciones usadas por el código; no implica que exista un proxy genérico hacia esos proveedores.
- **Exportar JSON:** descarga el catálogo documental, sin configuración ni credenciales del equipo.

Los campos encontrados en el código pueden pertenecer a la petición o la respuesta; no se presentan como un esquema OpenAPI completo. Consulta [MODELOS_API.md](MODELOS_API.md) y el código cuando prepares una modificación compleja.

### Probador

Selecciona una operación para preparar método, ruta, parámetros y ejemplo de cuerpo cuando esté disponible. En modo manual puedes escribir una ruta **registrada de este Gateway**; no se aceptan URLs arbitrarias. Usa tu sesión o un token de prueba, mantenido únicamente en memoria. Las modificaciones requieren confirmación y siguen sujetas a CSRF, roles y scopes.

El destino federado ofrece únicamente la consulta del catálogo de un peer aprobado. No reenvía tokens ni cookies al otro Gateway. Para su API administrativa abre el panel remoto y autentícate allí. La confianza de CA no es delegación administrativa.

Las peticiones se ejecutan una vez, sin reintentos; la respuesta está limitada a 1 MiB y 45 segundos en el visor. Un timeout no asegura que una modificación no se haya ejecutado. Revisa el estado antes de repetirla. Las descargas grandes deben realizarse con un cliente HTTP adecuado.

### Ejemplos de lectura

Define `HGW_TOKEN` de forma privada en tu terminal. Los ejemplos no incluyen credenciales reales.

```bash
curl --fail-with-body -H "Authorization: Bearer $HGW_TOKEN" \
  http://127.0.0.1:9091/api/v1/status
curl --fail-with-body -H "Authorization: Bearer $HGW_TOKEN" \
  http://127.0.0.1:9091/api/v1/network/nodes
```

```powershell
Invoke-RestMethod -Uri 'http://127.0.0.1:9091/api/v1/status' `
  -Headers @{Authorization="Bearer $env:HGW_TOKEN"}
```

El token para estado necesita `read`; el inventario de nodos necesita `network`. Un rol adecuado no evita la comprobación del ámbito. Las copias, portabilidad y determinadas acciones de confianza requieren sesión administrativa interactiva y no se habilitan con Bearer.

### Comandos locales

```bash
./headscale-gateway --help
./headscale-gateway version
./headscale-gateway doctor --db /ruta/gateway.sqlite
./headscale-gateway logs status --dir /ruta/logs
./headscale-gateway logs tail --channel gateway --lines 50 --dir /ruta/logs
./headscale-gateway logs follow --channel proxy --dir /ruta/logs
./headscale-gateway --db /ruta/gateway.sqlite --reload
```

En PowerShell sustituye `./headscale-gateway` por `.\headscale-gateway.exe`. `doctor`, `logs` y `version` no arrancan la VPN. `--reload` solicita recarga a la instancia que usa esa SQLite. No uses opciones internas de reinicio/restauración como comandos habituales. La opción antigua `--backup` sin cifrado está deshabilitada.

El inventario exportado de esta revisión se mantiene en [API_CLI.json](API_CLI.json). La versión desplegada puede ampliar ese índice; consulta siempre el panel correspondiente al servidor que utilizas.

## Identidad y permisos

El Gateway admite recuperación local, cuentas locales, LDAP y OIDC según configuración. Los grupos/correos del proveedor se asignan explícitamente a roles: autenticarse en un IdP no concede automáticamente administración. TOTP está disponible para la cuenta local.

Roles básicos: `viewer` para lecturas permitidas, `operator` para operaciones autorizadas y `admin` para administración. Cada endpoint puede imponer condiciones adicionales, como sesión interactiva, versión de la política o confirmación explícita.

Los scopes Bearer son `read`, `services`, `settings`, `network`, `operations` e `identity`. El rol, el scope y la política de exposición se comprueban conjuntamente. La credencial de recuperación no debe utilizarse como token de uso diario para la mini GUI.

## Copias, actualización y migración

Las copias se gestionan desde el panel, con cifrado y contraseña. Comprueba **qué contenido elegiste**: SQLite y clave, identidad tsnet, ACL, exportación DNS o conjunto completo. Una exportación DNS no significa restauración automática en el proveedor. Restaurar ACL es una acción independiente hacia Headscale.

### Actualizar sin perder configuración

1. Crea una copia cifrada desde el panel y verifica que puedes descargarla. Conserva su contraseña fuera del directorio del Gateway.
2. Anota la ruta `--db` y el usuario que ejecuta el proceso.
3. Detén el Gateway de forma normal.
4. Sustituye el ejecutable y los recursos que acompañen a la distribución. No reemplaces ni borres `gateway.sqlite`, `master.key`, la identidad tsnet ni los directorios de certificados.
5. Arranca con el mismo usuario y la misma ruta de datos.
6. Comprueba estado, servicios, certificados y acceso local antes de cerrar tu terminal de recuperación.
7. Recarga el panel con Ctrl+F5. La GUI tiene su distribución independiente.

Si necesitas una copia manual del directorio, hazla con el proceso detenido para conservar SQLite y su estado externo de forma coherente. No copies únicamente el archivo SQLite mientras está en escritura.

### Migrar y crear otro nodo

**Migrar** una instalación completa y **crear un segundo Gateway** son procedimientos diferentes. Para otro nodo utiliza datos e identidad nuevos. La exportación cifrada `.hgwcfg` transporta opciones y, si lo eliges, credenciales de proveedores; no clona la identidad tsnet ni las claves de CA. Revisa nombres, listeners y DNS antes de activarlo.

La restauración completa es una operación que puede reiniciar el proceso. Una respuesta 202 indica trabajo aceptado; consulta el estado posterior. Guarda las copias fuera del equipo que protegen.

### Guardar y recuperar desde Forgejo

1. Crea un repositorio **privado**, dedicado a los archivos cifrados del Gateway, e inicialízalo con una rama. Usa su URL HTTPS, por ejemplo `https://git.example.org/forgejo/equipo/copias-gateway`. El Gateway admite instalaciones bajo un prefijo como `/forgejo`.
2. Crea un token para ese repositorio con **`write:repository`** para enviar archivos. En **Sistema → Configuración** usa **Guardar perfil en Forgejo**, o en **Copias** usa **Guardar en Forgejo** en la fila de una copia existente. Un perfil nuevo requiere su propia contraseña. La copia existente conserva la que usabas para restaurarla.
3. En otro Gateway abre **Copias → Repositorio privado de Forgejo** e introduce la misma URL y un token con **`read:repository`**. Pulsa **Consultar repositorio**, selecciona el archivo e introduce su contraseña de cifrado. El perfil muestra los nombres de los ajustes antes de aplicarlos. Una copia se verifica y aparece en la lista local, desde la que puedes restaurar los componentes mediante los controles habituales.

Forgejo guarda cada archivo cifrado como adjunto de una publicación con SHA-256, fecha y tamaño. El Gateway verifica la privacidad del repositorio, el TLS y el SHA-256 del adjunto antes de abrirlo. El token se mantiene en el formulario hasta salir o recargar el panel. El motor lo recibe por operación y no lo guarda. La contraseña de cifrado se usa solo dentro del Gateway. Conserva la contraseña y un acceso de recuperación a Forgejo por separado.

El envío y la recuperación son manuales. La retención local no elimina adjuntos en Forgejo. El perfil admite hasta 320 KiB y la copia debe ser menor de 2 GiB. Forgejo o su proxy pueden aplicar límites inferiores. Si Forgejo está detrás de un portal SSO, tanto la API como la ruta de descarga de adjuntos deben llegar a Forgejo con el token; el Gateway rechaza redirecciones a páginas de inicio de sesión y a otros servidores.

## Registros e idiomas

Canales: `gateway.log`, `proxy.log` y `admin.log`. Niveles visibles en español: **ERROR, ADVERTENCIA, INFORMACIÓN, DEPURACIÓN y TRAZA**. En inglés: ERROR, WARNING, INFORMATION, DEBUG y TRACE. Los códigos técnicos de filtro son `error`, `warn`, `info`, `debug` y `trace`.

El panel permite filtrar por canal, nivel e identificador de evento y configurar la presentación horaria. El nivel de registro puede definirse por CLI, `GATEWAY_LOG_LEVEL` o SQLite, en ese orden. La depuración temporal ayuda a limitar el volumen.

El panel y la GUI tienen selección de idioma. Los mensajes nuevos del servidor usan `GATEWAY_LANG`; el portal atiende al idioma solicitado por el navegador. Los textos personalizados del usuario no se traducen automáticamente.

Para añadir un idioma: crea `src/locale/<codigo>.po`, traduce con Poedit u otra herramienta gettext, ejecuta `python3 tools/extract_i18n.py`, valida con `python3 tools/check_i18n.py` y recompila el servidor/cliente. Consulta [IDIOMAS_Y_REGISTROS.txt](IDIOMAS_Y_REGISTROS.txt).

## Compilar y verificar

Desde la raíz del proyecto:

```bash
python3 tools/reference_catalog.py
python3 tools/extract_i18n.py
python3 tools/check_i18n.py
./tools/verify.sh
./build.sh
# PowerShell: .\build.ps1
```

Para la GUI:

```bash
cd src/desktop
npm ci
npm test
npm run pack:all
```

El empaquetador genera Windows x64 y Linux x64 en `src/desktop/dist` y actualiza `bin/Gateway-Tray` y `bin/linux/Gateway-Tray`. Necesita Python 3.11 o posterior y descarga dependencias/Electron y los fuentes correspondientes del runtime (unos 4,5 GB, reutilizados desde una caché fuera de Git). Cada escritorio incluye esos fuentes y avisos en `legal/`; el publicador verifica su integridad antes de generar una entrega. Consulta [fuentes y licencias de escritorio](ESCRITORIO_LICENCIAS.md). Las pruebas visuales en `src/tools/gui-linux-check` requieren una sesión gráfica Linux; no forman parte de una ejecución CI sin escritorio. Para regenerar el fixture documental usado por la prueba visual, ejecuta:

```bash
HGW_REFERENCE_EXPORT=/tmp/hgw-reference-catalog.json go -C src test ./internal/admin -run TestReferenceMatchesRegisteredRoutes -count=1
src/desktop/node_modules/electron/dist/electron src/tools/gui-linux-check/panel.cjs
```

Las CI están en `.forgejo/workflows/quality.yml` y `.github/workflows/quality.yml`. Las pruebas automatizadas usan datos temporales y servidores simulados; no necesitan tus claves de producción. La comprobación de dos gateways reales y del almacén privilegiado del SO se realiza por separado.

En ambos servidores, cada `push` y cada `pull_request` ejecuta también el trabajo
**Seguridad de dependencias**, con las mismas auditorías Go y npm descritas a
continuación. El resultado aparece antes de fusionar el cambio. Para exigirlo
como condición de fusión, añade ese estado a la protección de `main` del servidor;
los workflows no modifican esa configuración.

## Publicar el complemento en Mis-app

Añade [Mis-app](https://github.com/Gente-del-Condado/Mis-app) en **Ajustes → Aplicaciones → Instalar aplicación → Repositorios** de Home Assistant y selecciona **Headscale Gateway**.

Cuando la réplica de Forgejo lleva a GitHub un cambio de `addons/headscale_gateway/` en `main`, GitHub Actions analiza la seguridad, valida los paquetes generados y sincroniza esa carpeta en la raíz de Mis-app. Los cambios en los manifiestos Go y npm también activan el flujo. También puedes lanzar **Publicar Headscale Gateway en Mis-app** manualmente desde Actions. La publicación conserva las demás aplicaciones. Para que Supervisor ofrezca una actualización, incrementa `VERSION` con `tools/set_version.py` y regenera los paquetes antes de subir los cambios.

Antes de acceder a la clave de publicación, el flujo ejecuta `govulncheck` fijado
en `v1.9.0`, primero sobre todos los módulos y después sobre las llamadas del
código y la biblioteca estándar. Ejecuta también `npm ci --ignore-scripts
--no-audit` y `npm audit --include=dev --audit-level=low` en `src/desktop/`.
La auditoría incluye herramientas de desarrollo y dependencias transitivas.
Los analizadores consultan sus bases de avisos actuales; cualquier vulnerabilidad
detectada, error de descarga o fallo del análisis bloquea la sincronización.
Consulta los pasos fallidos en Actions, corrige las dependencias y vuelve a
subir el cambio. No se ejecuta `audit fix` ni se descartan avisos automáticamente.

Para repetir los análisis desde la raíz:

```bash
go -C src run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -scan module
go -C src run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -show verbose ./...
npm --prefix src/desktop ci --ignore-scripts --no-audit
npm --prefix src/desktop audit --include=dev --audit-level=low
```

La publicación automática usa una clave SSH dedicada de Forgejo. Registra su
parte pública con permiso de escritura en **Mis-app → Configuración → Claves
de despliegue** de Forgejo. Guarda la parte privada como secreto
`MIS_APP_FORGEJO_DEPLOY_KEY` en **headscale-gateway → Settings → Secrets and
variables → Actions** de GitHub. El flujo publica por SSH en el catálogo de
Forgejo, cuya réplica actualiza después Mis-app en GitHub.

## Publicar el paquete YunoHost

El repositorio público de instalación es
[headscale-gateway_ynh](https://github.com/Gente-del-Condado/headscale-gateway_ynh).
Consulta la [guía de YunoHost](../packaging/yunohost/doc/ADMIN.md) para instalarlo y
actualizarlo mediante su URL. El paquete sigue siendo experimental hasta
completar la aceptación en una máquina virtual real; publicarlo no lo incorpora
al catálogo oficial de YunoHost.

La cadena de publicación es:

1. Fusionar el cambio en `main` del proyecto principal de Forgejo tras superar
   **Seguridad de dependencias**, exigida por la protección de la rama.
2. La réplica existente actualiza `Gente-del-Condado/headscale-gateway` en GitHub.
3. El flujo **Publicar el paquete YunoHost** audita Go y npm con las mismas
   comprobaciones anteriores y valida `git-install-manifest.json`. Solo entonces
   obtiene la clave y publica por SSH en
   `wolfbelmi88/headscale-gateway_ynh` de Forgejo.
4. El espejo de salida de ese repositorio actualiza el repositorio público
   `Gente-del-Condado/headscale-gateway_ynh` de GitHub.

El flujo se activa al cambiar `yunohost/`, las dependencias auditadas o el propio
publicador en `main`. Usa el último `main`, serializa las publicaciones y no
crea commits si el paquete ya coincide. No se publica desde los pull requests.
También admite ejecución manual para recuperar una sincronización fallida.

`tools/publish_yunohost_repo.py` exporta solo el árbol **versionado**
`HEAD:yunohost` a la raíz de otro checkout. Comprueba versión, manifiestos,
sumas de los dos binarios y sus permisos antes de sustituir los archivos
administrados. Rechaza enlaces y rutas ajenas al paquete. Conserva los archivos
ajenos del destino, las licencias y los avisos; no copia el historial privado,
`src/`, el complemento Home Assistant ni los datos de instalaciones.

Para configurar las credenciales, registra una clave de despliegue dedicada
con escritura solo en **headscale-gateway_ynh de Forgejo** y guarda su parte
privada como secreto `YNH_FORGEJO_DEPLOY_KEY` en **headscale-gateway → Settings →
Secrets and variables → Actions** de GitHub. Configura en el repositorio de
paquete de Forgejo un espejo SSH hacia
`git@github.com:Gente-del-Condado/headscale-gateway_ynh.git`, sincronización al
recibir commits e intervalo de respaldo. Registra la clave pública de ese espejo
como clave de despliegue con escritura exclusivamente en el paquete de GitHub.
Son dos claves distintas; ninguna necesita escritura en el proyecto principal.

No cambies la versión para repetir una publicación idéntica. Para una nueva
versión del motor usa `tools/set_version.py`, regenera los paquetes y fusiona
el cambio en Forgejo. La sincronización del repositorio no ejecuta actualizaciones
en los servidores YunoHost instalados.

## Publicar en Forgejo o GitHub

Mantén el proyecto en tu **repositorio privado**, con las fuentes y las carpetas
instalables preparadas mediante `python3 tools/prepare_git_install.py`.
`addons/headscale_gateway/` se copia a Home Assistant y `yunohost/` se instala
desde un clon local. Sus archivos generados, incluidos los binarios necesarios
de `yunohost/payload/`, sí se incluyen en Git para permitir actualizar con
`git pull` sin trasladar ZIP ni compilar Go en YunoHost. Consulta
[Instalación privada](INSTALACION_PRIVADA.md).

Las compilaciones generales de `bin/` y los archivos de `releases/` permanecen
fuera del historial. Si necesitas distribuciones independientes,
`tools/package_release.py` prepara los archivos de entrega y excluye los datos
de ejecución y las credenciales.

Incluye `src/go.mod`, `src/go.sum`, los lockfiles, fuentes, pruebas, traducciones, scripts y documentación. No incluyas SQLite, `master.key`, claves privadas, perfiles de la mini GUI, archivos `.hgw/.hgwcfg`, logs reales ni tokens. `.gitignore` ayuda con los archivos nuevos, pero no retira secretos que ya estuvieran versionados.

Antes de vender, completa las condiciones comerciales y el cumplimiento de dependencias descrito en [LICENCIAS.md](LICENCIAS.md). El runner de Forgejo ejecuta las comprobaciones; los flujos de publicación de GitHub solo actualizan Mis-app y el paquete YunoHost mediante las claves dedicadas descritas arriba.

## Solución de problemas

| Síntoma | Comprobación |
| --- | --- |
| La GUI conecta con read+network, pero no con read solo | El inventario requiere el ámbito network además del estado |
| Error 401 | Token/sesión incorrecto, caducado o ausente |
| Error 403 | Rol, scope, política Bearer o requisito de sesión/CSRF |
| Catálogo remoto 404 | Publicación desactivada, versión antigua o URL que no es el portal principal |
| Certificado inválido | CA no confiada, hostname/IP fuera del SAN, fecha o resolución DNS |
| Servicio HTTPS interno falla por CA desconocida | Revisa la confianza TLS hacia el backend; no es necesariamente la CA del portal |
| El portal remoto no aparece | Peer no aprobado, servicio no compartido/seleccionado, fallo TLS/ACL o caché caducada |
| DNS resuelve pero no conecta | Comprueba puerto, listener, rutas, ACL y firewall; DNS no prueba conectividad |
| Pausa bloqueada desde VPN | Abre administración local/LAN para evitar cortar la sesión que utilizas |
| No aparece bandeja/aviso en Linux | Soporte del escritorio, notificaciones habilitadas y proceso en ejecución |
| SQLite ocupada | Descarta instancias simultáneas y revisa logs; no borres la base de datos |
| Se guardó configuración pero no cambió | Comprueba si el panel indica reinicio pendiente |

## Límites y licencia

No se implementa administración remota automática por confiar una CA, descubrimiento irrestricto de toda la zona DNS, instalación remota de CA en clientes, CRL/OCSP de certificados emitidos ni restauración automática de la configuración completa del VPS Headscale. Los límites de tamaño, puertos y caducidad del catálogo están en la guía de federación.

Las pruebas del helper privilegiado y el recorrido con un segundo Gateway real siguen siendo comprobaciones separadas. Una compilación correcta para Windows/macOS no equivale a ejecutar allí esta revisión.

**Código propio bajo AGPL-3.0-only**, según [LICENSE](../LICENSE) y [NOTICE](../NOTICE). Las dependencias conservan sus licencias y derechos. El módulo de activación por suscripción está desactivado en las compilaciones normales; se habilita antes de compilar con una clave pública del emisor. Consulta [SUSCRIPCIONES.md](SUSCRIPCIONES.md). Los paquetes de escritorio incorporan los fuentes y avisos del runtime descritos en [LICENCIAS.md](LICENCIAS.md); conserva la carpeta `legal/` al redistribuirlos.

Documentación complementaria: [arquitectura](ARQUITECTURA.md), [pruebas](PRUEBAS.md), [CA y federación](CA-FEDERACION.md), [portal](PORTAL_Y_FEDERACION.txt), [helper](HELPER_CA.txt) e [idiomas](IDIOMAS_Y_REGISTROS.txt).


## Estructura y entregas

El mapa vigente está en [ESTRUCTURA.md](ESTRUCTURA.md). El motor, el panel y
el cliente de escritorio están en `src/`; Android en `android/src/`.
Los comandos de desarrollo y compilación se explican en el
[README del repositorio](../README.md).
Las entregas se generan en carpetas nuevas de `releases/`. Los paquetes listos
para instalación privada se conservan en `addons/` y `yunohost/`.
No se incluyen bases de datos, credenciales, perfiles ni cachés.
