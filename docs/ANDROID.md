# Headscale Gateway para Android: arquitectura, interfaz y validación

Estado de publicación: 1.4.7. Las pruebas del 4 y 5 de noviembre que siguen
son antecedentes y no sustituyen la aceptación de cada nueva compilación.

La entrega 1.4.7 comparte el motor Go, apunta a API 36 y prepara APK firmado,
APK sin firma para F-Droid y AAB para Play. La clave estable queda fuera de Git.
Consulta [los canales](../packaging/android/README.md) y la
[privacidad](PRIVACIDAD_ANDROID.md).

Revisión histórica del 5 de noviembre de 2026. Objetivo confirmado: ejecutar el Gateway
completo en Android, con su propia conexión a Headscale y publicación de
servicios. No se limita a administrar un servidor remoto.

## Implementación y pruebas

La aplicación experimental está en `android/`. Ejecuta el motor común como
PIE, con una única identidad `tsnet`, el mismo catálogo SQLite, panel, API,
certificados y proxy de las otras plataformas. El túnel del teléfono utiliza
`VpnService`, HEV y un SOCKS5 autenticado del mismo nodo. No requiere root.

Se generan APK para arm64 y x86_64, API mínima 24. Los segmentos ELF del motor
y del adaptador nativo están alineados a 16 KiB. Esto no sustituye una prueba
en un dispositivo real con páginas de 16 KiB.

El 4 de noviembre de 2026 se verificó en Waydroid, Android 13 x86_64:

- Arranque del motor y acceso normal al panel con una sesión administradora.
- Rechazo del adaptador privado cuando falta su credencial.
- Persistencia de la identidad y reinicio del motor al aplicar la configuración.
- Registro y conexión a un Headscale 0.29.7 temporal, separado del servidor real.
- Publicación de un servicio HTTPS, abierto desde un segundo nodo `tsnet`.
- Conexión TCP desde un proceso Android ajeno al UID del Gateway hacia ese nodo,
  utilizando el TUN del teléfono.
- Tráfico UDP entre el teléfono y el segundo nodo.
- Consultas directas de MagicDNS por el DNS de la VPN.
- Cálculo de rutas IPv4/IPv6 y exclusión de loopback y de la LAN.
- Verificación general del repositorio mediante `tools/verify.sh`.

La primera ejecución detectó que el controlador SQLite de escritorio utilizaba
una llamada bloqueada por seccomp de Android. Android selecciona ahora
`mattn/go-sqlite3` con CGO/Bionic; las demás plataformas conservan `modernc`.
El esquema, las transacciones, los PRAGMA y las copias siguen siendo comunes.

La prueba utiliza una clave de alta efímera y una CA local de laboratorio.
El cliente auxiliar verifica la cadena y el nombre HTTPS usando la CA pública
del dominio activo. No se omite la validación TLS, tampoco en el laboratorio. Ni la clave ni la APK de
instrumentación se incorporan a la APK principal.

La prueba también detectó que `tsnet.Dial` a quad-100 devolvía SERVFAIL,
aunque el resolvedor interno sí respondía. El adaptador Android entrega las
consultas TCP/UDP directamente a ese gestor DNS. Se mantienen los resolutores,
las rutas y las políticas del nodo.

Waydroid tiene DNS privado estricto hacia un proveedor externo. Las consultas
directas a MagicDNS pasan, pero la resolución de nombres del sistema Android
sigue consultando a ese proveedor y falla para nombres privados. Ese ajuste
se conserva; la aplicación avisa y ofrece abrir los ajustes DNS. No se cuenta
la resolución del sistema con DNS privado estricto como prueba superada.

Instrucciones: [compilación, instalación y uso](../android/README.md).
Laboratorio reproducible: [pruebas Android](../android/PRUEBAS.md).

## Interfaz móvil (android.2)

Se reemplaza la columna de controles de la primera APK por Inicio, Panel y
Ayuda. El panel dispone del espacio central completo; su menú Secciones se
construye con las opciones reales del panel común. El contenedor añade una
presentación móvil local, sin cambiar las API, permisos ni formularios comunes.
Se mantienen los datos al actualizar con la misma firma.

La revisión `android.2` pasó el 5 de noviembre de 2026 las pruebas instrumentadas
en Waydroid: sesión real del WebView, carga de la presentación móvil con CSP,
menú de las secciones habilitadas, navegación a Servicios, retorno a Inicio y
Ayuda, ausencia de desbordamiento horizontal en las secciones visibles y más del 60 % de la altura de la ventana para el panel. Se comprobó una ventana especialmente
estrecha, con controles táctiles de al menos 48 dp en la navegación nativa.
No se cambió la VPN activa del dispositivo para estas pruebas de interfaz.

El estado de la VPN del teléfono se comunica desde su proceso auxiliar mediante
un archivo privado y se comprueba que el proceso siga vivo. Así no se confunde
la conexión del nodo con la conexión de las aplicaciones ni se depende de la
caché de preferencias de otro proceso.

## Revisión funcional (android.3)

El 5 de noviembre de 2026 se ejecutó la APK `.lab` en Waydroid Android 13,
separada de los datos de la aplicación habitual. Headscale 0.29.7 utilizó HTTPS,
una CA temporal y claves de corta duración. El proveedor de documentos pertenece
al paquete de instrumentación y verifica la firma de quien solicita acceso;
no se incorpora a la APK normal.

| Función | Resultado comprobado |
| --- | --- |
| Configurar Headscale | HTTPS, clave API, inventario de nodos y rechazo de URL inválida sin alterar la configuración |
| Publicar servicios | Crear, editar, borrar y recuperar el catálogo exacto tras restaurar |
| Certificados | CA X.509, vigencia y huella, exportación pública sin claves privadas y HTTPS desde otro nodo validando cadena y nombre |
| Archivos Android | Descarga con nombre original, blob revocado inmediatamente, guardado y selección mediante ContentResolver y proveedor de prueba |
| Copias | Configurar cifrado, rechazar contraseña incorrecta, crear copia SQLite parcial y copia completa, guardar programación y verificar SHA-256 |
| Restauración | Importar el archivo elegido, eliminar un servicio y cambiar ajustes, restaurar realmente, reconectar y comprobar identidad VPN y CA originales |
| Compatibilidad Linux | Descifrar `.hgwcfg` y las copias parcial/completa con el mismo formato y leer SQLite/secreto con `modernc` en Linux |
| VPN | Tras restaurar: HTTPS publicado, TCP y UDP del teléfono, consulta directa de MagicDNS y estado del proceso auxiliar |
| Regresión | `tools/verify.sh` y paquetes privados regenerados y comprobados |

Las correcciones incluyen transferir directamente los objetos Blob del panel
sin debilitar CSP, conservar el nombre de archivo, recuperar exportaciones
pendientes y mantener el WebView y sus selectores al girar. Los temporales se
ubican en la caché privada de Android. Certificados, descubrimiento y diagnóstico
identifican Android y no intentan utilizar herramientas privilegiadas de Linux.

Android Lint pasa sin errores con `minSdk 24`: el registro del proceso evita
API exclusivas de Android 8 y la lectura del MTU utiliza una alternativa antes
de Android 10. Se volvió a comprobar el arranque, la sesión y la navegación
en Android 13 después de estos cambios. Esto no sustituye las pruebas de
ejecución pendientes en Android 7–9 ni en un teléfono físico.

## Arquitectura implementada

```text
Interfaz Android en español y WebView del panel común
        │ sesión normal, inicio, parada y selector de archivos
        ▼
GatewayService ── proceso Go común de Headscale Gateway
        │            ├── gateway.sqlite, master.key e identidad tsnet
        │            ├── panel, API, portal y federación
        │            ├── proxy HTTPS, certificados y DNS
        │            └── adaptador loopback autenticado: estado y SOCKS5
        │
GatewayVpnService, proceso auxiliar :vpn
        └── VpnService TUN → HEV → SOCKS5 → el mismo nodo tsnet
```

La aplicación queda fuera del túnel para evitar bucles. Los destinos se
resuelven mediante el nodo común, incluidas las rutas aceptadas. Sin nodo de
salida, Internet utiliza la conexión habitual a través del adaptador. El túnel
transporta TCP y UDP; ICMP no forma parte de SOCKS5.

Se conserva la LAN cuando no hay nodo de salida o cuando sus preferencias
permiten acceso LAN. Loopback y multicast quedan excluidos. Reinicia la VPN
del teléfono tras cambiar el nodo de salida, el permiso LAN o la red física:
la tabla de exclusiones se calcula al establecer el TUN.

El motor recibe las interfaces físicas mediante un archivo privado actualizado
por Android; no depende de netlink, restringido en el sandbox. Si muere su
proceso padre, el motor recibe SIGTERM para evitar procesos huérfanos. La parada
del túnel se aísla en un proceso auxiliar y tiene un límite de espera.

## Qué se reutiliza de TailSocks

Se estudió la copia local 5.6.0, sus servicios Kotlin/Compose, `appctr`, el
ciclo de vida del ejecutable, la enumeración de red y su adaptación VPN.
No se modifica ese proyecto ni se ejecuta su `tailscaled` junto al Gateway.
Se reutiliza su copia de `hev-socks5-tunnel` con las dependencias y licencias
originales; el contenedor Android y el JNI son propios de este proyecto.
Consulta [procedencia y licencias](../android/TERCEROS.md).

## Validación pendiente

La APK es experimental. El modo «VPN siempre activa» queda desactivado en el
manifiesto porque el túnel necesita iniciar primero el Gateway. Aún requieren pruebas un teléfono arm64 físico,
Android 14–16, páginas de 16 KiB en ejecución, cambios Wi-Fi/datos, suspensión,
recuperación tras cierre forzado, nodos de salida y rutas de subred reales.
También quedan por validar los flujos LDAP/OIDC, los proveedores DNS/ACME,
la selección en proveedores de documentos reales de distintos fabricantes, las
exportaciones de gran tamaño y una actualización con firma estable. La
restauración completa ya pasó con un proveedor Android de laboratorio. IPv6 requiere una prueba de tráfico de extremo a extremo.

No se garantiza disponibilidad permanente: Android puede detener servicios
por restricciones de batería. Solo puede haber una VPN activa por perfil.
En Waydroid se solicita autorización para sustituir otra VPN y se restaura
su configuración después de la prueba. Los emuladores del SDK instalados en
este equipo fallaron antes del arranque; no se cuentan como pruebas superadas.

## Identidad, español y distribución privada

El producto derivado se llamará **Headscale Gateway para Android**, con su
propio identificador de aplicación y firma. La actualización del producto
debe conservar esa firma. Los identificadores nativos, proveedores de archivos,
enlaces y actualizador deben corresponder a nuestra aplicación.

TailSocks contiene 88 archivos de cadenas y unas 4.800 entradas en su catálogo
predeterminado; no tiene un catálogo `values-es/` en la copia revisada.
Cambiar solo el nombre de la aplicación dejaría la interfaz incompleta en español.
Las pantallas incorporadas, notificaciones, permisos explicados, errores,
ayuda y documentación propios deben estar en español, incluido el idioma
predeterminado. Las funciones ajenas al Gateway no se incorporan automáticamente.

El código de TailSocks y las bibliotecas que se reutilicen conservan sus
licencias y avisos originales. La explicación y la documentación del producto
serán españolas; los textos legales originales se mantienen como avisos.
No se atribuye al proyecto la autoría exclusiva de componentes de terceros.

Forgejo privado seguirá siendo el repositorio de desarrollo. El APK podrá
distribuirse de forma privada; no es necesario publicarlo en Google Play.
El actualizador original de TailSocks no debe descargar sus APK como si fueran
actualizaciones de nuestra aplicación. Las claves de firma y credenciales de
Forgejo permanecen fuera del repositorio y del APK.

## Compilación del motor y la APK

`tools/build_android_engine.py` compila el motor y comprueba la alineación ELF.
`tools/build_android_apk.py` lo incorpora a la aplicación junto con los avisos
de terceros y genera la suma SHA-256. Las instrucciones completas, requisitos
y variables de firma están en [android/README.md](../android/README.md).
No se precisa publicar Forgejo ni introducir sus credenciales en la APK.

La instrumentación de `android/src/androidTest/` es un paquete separado.
Comprueba el motor, la autenticación y las rutas; su opción de laboratorio
permite configurar un Headscale temporal y un servicio de prueba. El auxiliar
`tools/android-lab-peer/` comprueba el proxy desde otro nodo y ofrece destinos
TCP/UDP para probar el túnel. Nunca debe ejecutarse contra una base de datos
o una tailnet de producción.

## Referencias oficiales

- [Servicios VPN de Android](https://developer.android.com/develop/connectivity/vpn).
- [Tipos de servicios en primer plano](https://developer.android.com/develop/background-work/services/fgs/service-types).
- [Restricciones de inicio en segundo plano](https://developer.android.com/develop/background-work/services/fgs/restrictions-bg-start).
- [Compatibilidad con páginas de 16 KiB](https://developer.android.com/guide/practices/page-sizes).

Fuentes locales contrastadas: `tailscale.go`, `vpn_supervisor.go`,
`startup_options.go`, `admin_server.go`, el código de `tailscale.com/tsnet`
fijado en `src/go.mod`, y los archivos de TailSocks indicados en este documento.
