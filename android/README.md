# Headscale Gateway para Android

Aplicación experimental con el motor completo del proyecto, panel administrativo
en español y conexión de las aplicaciones del teléfono a la misma VPN.
No necesita root ni otro cliente Tailscale instalado. La descarga directa utiliza una firma estable. Se preparan también variantes
para F-Droid y Google Play; no están publicadas en esas tiendas. F-Droid exige
los fuentes completos en un repositorio público.

## Uso

1. Instala la APK y abre **Headscale Gateway**.
2. Pulsa **Iniciar Gateway** y permite las notificaciones para ver su estado.
3. Abre **Panel** y usa **Secciones → Configuración** para configurar Headscale,
   el dominio y los certificados. Los
   cambios que lo requieran se aplican con **Reiniciar Gateway**, dentro del panel.
4. Autoriza el nodo en tu Headscale mediante el enlace de registro HTTPS.
5. Añade los servicios desde el panel y comprueba DNS y certificados desde otro nodo.
6. Para conectar además el teléfono, pulsa **Conectar aplicaciones**
   y acepta el permiso Android. Esto sustituye cualquier otra VPN del perfil.

La navegación inferior conserva tres pantallas:

- **Inicio**: estado del Gateway, direcciones del nodo, servicios configurados,
  conexión de las aplicaciones y accesos a la administración.
- **Panel**: administración común a pantalla amplia. **Secciones** muestra las
  opciones habilitadas para tu sesión, incluidas plantillas, certificados,
  copias, acceso, API y herramientas Headscale. No hay una segunda configuración
  Android de servicios o credenciales de Headscale.
- **Ayuda**: datos de acceso inicial, diagnóstico, DNS privado, ajustes Android
  y avisos de terceros.

El panel adapta formularios y tablas al ancho del móvil. Su hoja de estilo
Android se sirve dentro del WebView con el mismo origen, conservando la política
CSP y la autenticación del panel. Linux, Windows, Home Assistant y YunoHost
siguen usando el panel compartido habitual. Se conserva la pantalla nativa
seleccionada al girar; los datos se guardan desde el panel. Salir de la actividad
no detiene el nodo.

El nodo, el proxy y el túnel del teléfono comparten una identidad. El proceso
del Gateway queda fuera del túnel para evitar bucles. El adaptador lleva TCP
y UDP a ese nodo; los destinos de la tailnet y las rutas aceptadas se resuelven
por él. Sin nodo de salida, Internet utiliza la conexión habitual del teléfono
a través del adaptador; activar la VPN no convierte por sí solo el teléfono
en un servicio de salida anónima.

La red local queda fuera del túnel cuando lo permiten las preferencias del
nodo de salida. Reinicia la VPN del teléfono al cambiar esas preferencias o
la red física. El adaptador transporta TCP y UDP; no implementa ICMP.

**Desconectar aplicaciones** conserva el Gateway y sus servicios.
**Detener todo** detiene ambos. Android puede detener la aplicación por las
restricciones de batería o por una parada forzada; no se promete disponibilidad
permanente como la de un servidor dedicado. Esta revisión se inicia desde la
aplicación y no ofrece «VPN siempre activa» ni bloqueo de todas las conexiones
al arrancar: ese modo necesita un ciclo de inicio independiente que todavía
no está implementado.

## Acceso, archivos y certificados

La cuenta local inicial se llama `android` y tiene una contraseña aleatoria
protegida mediante Android Keystore. **Datos de acceso inicial** permite verla.
La aplicación inicia una sesión normal del panel; no elimina su autenticación.
Si cambias la contraseña o el proveedor de identidad, utiliza tus credenciales
actuales. Los flujos LDAP/OIDC del panel requieren pruebas específicas en Android.

La SQLite, `master.key`, los certificados y el estado VPN se guardan en el
almacenamiento privado. No se incluyen credenciales de Forgejo ni del usuario
en el APK. Las copias automáticas del sistema están desactivadas.
Los secretos del adaptador no se envían al panel ni se incluyen en enlaces.

El selector de archivos sirve para importar y exportar desde el panel. Las
descargas se guardan donde el usuario elija. Se transfieren por bloques y se
comprueba que haya espacio para el temporal y para guardar el resultado; no hay
un límite fijo de 512 MiB. Una exportación preparada se conserva si se recrea
la actividad y puede guardarse o descartarse al volver a abrirla. La instalación de una CA en el almacén del sistema Android requiere
el procedimiento y consentimiento de Android; el helper polkit de Linux no
forma parte del APK. El DNS privado estricto puede enviar los nombres privados a un proveedor externo
y evitar MagicDNS; en Waydroid se comprobó este comportamiento. La aplicación
muestra un aviso y permite abrir el ajuste de Android, sin cambiarlo por sí sola.
Un cortafuegos también puede afectar la conexión. Conserva la validación TLS.

## Configuración, servicios y copias

La adaptación del motor Go compartido, introducida en `android.3`, conserva las API y formatos comunes. Se han comprobado
Headscale por HTTPS con clave API, inventario de nodos, creación/edición/borrado
de servicios, generación y descarga de la CA, perfiles `.hgwcfg` y copias `.hgw`.
Las copias parcial y completa creadas en Android se han descifrado y leído con
el controlador SQLite de Linux, incluyendo sus secretos.

Para una copia utiliza **Panel → Secciones → Copias de seguridad**. Configura
una contraseña de recuperación y selecciona los componentes. Guarda la descarga
fuera de la aplicación. La importación verifica la copia y su contraseña; solo
**Restaurar desde panel** aplica la restauración y reinicia el motor.

La restauración completa conserva los servicios, los certificados y la identidad
VPN incluidos. Las ACL remotas de Headscale se restauran mediante su operación
explícita, con sus confirmaciones; no se reemplazan automáticamente al recuperar
los datos locales. Si restauras una copia de otro equipo, utiliza la cuenta y
el método de acceso guardados en esa copia. Los datos iniciales de acceso que
muestra Android no sustituyen la contraseña restaurada.

La instalación de una CA raíz en Android se realiza desde sus ajustes, con
consentimiento del usuario. El panel ofrece **Instalar en Android** y la descarga
del certificado público. No intenta ejecutar `sudo`, `pkexec` ni herramientas
de escritorio. Este es el procedimiento admitido por
[Android para las CA](https://developer.android.com/reference/android/security/KeyChain#createInstallIntent()).

## Compilar

Requisitos: Go de `src/go.mod`, Python 3, JDK 17, Gradle 8.13, SDK Android API 36
y NDK 27.0.12077973. El código Android utiliza el motor común de `src/`.

```bash
python3 tools/build_android_apk.py \
  --sdk "$ANDROID_HOME" \
  --ndk "$ANDROID_NDK_HOME" \
  --java-home "$JAVA_HOME" \
  --gradle gradle
```

La salida es `builds/android/headscale-gateway-debug.apk`, con arm64 y x86_64.
Con `--arch arm64-v8a` se limita al teléfono; con `--arch x86_64`, al laboratorio.
Las compilaciones de pruebas usan el identificador
`org.headscalegateway.android.dev` para coexistir con una futura entrega estable.
No se añaden las carpetas de compilación ni las claves a Git.

## Firma estable y tiendas

Crea una identidad una sola vez, fuera del repositorio:

```sh
python3 tools/android_signing.py init "$HOME/.local/share/headscale-gateway-signing/android"
```

Respalda toda esa carpeta de forma segura. Para actualizar la descarga directa
se necesita la misma clave; no la vuelvas a generar en cada versión. El perfil
privado se carga únicamente para el proceso de construcción, sin contraseñas
en los argumentos de la orden.

```sh
python3 tools/build_android_apk.py --release --channel direct \
  --sdk "$ANDROID_HOME" --ndk "$ANDROID_NDK_HOME" --java-home "$JAVA_HOME" \
  --gradle "$PWD/android/gradlew" \
  --signing-config "$HOME/.local/share/headscale-gateway-signing/android/signing.json"
```

La salida firmada es `builds/android/headscale-gateway-release.apk`. La herramienta
verifica identidad, versión, SDK, firma, ausencia de depuración y alineación de
16 KiB de todas las bibliotecas nativas para ambas arquitecturas. Guarda un
informe `.verification.json` y una suma `.sha256` junto al artefacto.

Para Play usa `--channel play --bundletool /ruta/bundletool.jar`, conservando
el perfil de firma; genera `headscale-gateway-play.aab`. Para F-Droid utiliza
`--release --channel fdroid`, sin perfil: genera
`headscale-gateway-fdroid-unsigned.apk`. Este APK sin firma es para el constructor
de F-Droid y no se instala como entrega directa. Los canales pueden usar firmas
diferentes; cambiar de canal puede requerir copia, desinstalación e importación.
Consulta [los canales y sus requisitos](../packaging/android/README.md) y la
[política de privacidad](../docs/PRIVACIDAD_ANDROID.md).

## Verificación y límites

Se compilan ambas arquitecturas y se comprueba la alineación ELF de 16 KiB.
La instrumentación de `android/src/androidTest/` verifica el arranque real, la
sesión del panel y el rechazo del adaptador sin credenciales. Incluye una
prueba opcional con un Headscale y otro nodo temporales, TCP, UDP y MagicDNS.
Consulta [los pasos del laboratorio](PRUEBAS.md). La APK de pruebas
instrumentadas se construye aparte y no se distribuye como aplicación.

Waydroid requiere desactivar temporalmente su suspensión automática durante
las pruebas largas, y restaurar el valor previo al terminarlas. No se borran
sus datos ni se desinstalan otras aplicaciones. Una prueba de túnel exige
autorización si hay otra VPN activa.

El estado de aceptación, la arquitectura y el trabajo pendiente se registran
en [la documentación Android](../docs/ANDROID.md). La versión sigue siendo
experimental hasta validar también un teléfono físico, reconexión Wi-Fi/datos,
suspensión, IPv6 y proveedores DNS/ACME reales.

Consulta [los componentes reutilizados y sus avisos](TERCEROS.md).
