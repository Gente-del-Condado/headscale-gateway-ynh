# Laboratorio funcional de Android

Las pruebas con escrituras utilizan **`org.headscalegateway.android.lab`** y
un paquete de instrumentación separado. Coexisten con la APK habitual `.dev`.
El ejecutor rechaza otros identificadores. No uses una base de datos, un
Headscale ni credenciales de producción.

## Construcción e instalación

Desde la raíz, con el SDK, NDK, JDK 17 y Gradle indicados en `README.md`:

```bash
python3 tools/build_android_apk.py --lab \
  --sdk "$ANDROID_HOME" --ndk "$ANDROID_NDK_HOME" \
  --java-home "$JAVA_HOME" --gradle gradle

gradle -p android -PlabBuild=true :assembleDebugAndroidTest

adb -s "$ANDROID_SERIAL" install -r builds/android/headscale-gateway-laboratorio.apk
adb -s "$ANDROID_SERIAL" install -r -t \
  android/build/outputs/apk/androidTest/debug/HeadscaleGatewayAndroid-debug-androidTest.apk
```

El paquete de instrumentación contiene un proveedor de documentos protegido
por la firma de las APK. Comprueba el contrato del selector Android y los bytes
que se guardan o importan mediante `ContentResolver`. La selección visual en
proveedores de distintos fabricantes requiere pruebas adicionales en un móvil.
El proveedor, las claves y las pruebas no están en la APK normal.

## Servidor y segundo nodo

Prepara un Headscale 0.29.7 temporal con estas condiciones:

- HTTPS accesible desde Android (`HEADSCALE_LAB_URL`) y desde el equipo de
  pruebas (`HEADSCALE_LAB_PEER_URL`). El certificado debe cubrir ambos nombres
  o direcciones. Conserva su CA pública en `HEADSCALE_LAB_CA_FILE`.
- MagicDNS con dominio `lab.example.test`, un usuario de laboratorio y permiso
  para que sus dos nodos se comuniquen.
- Política ACL inicializada, por ejemplo en modo `database`. Una política en
  modo `file` sin archivo produce errores al pedir una copia completa de ACL.
- Una clave de registro reutilizable y de corta duración en
  `HEADSCALE_LAB_KEY_FILE`, y una clave API efímera en `HEADSCALE_LAB_API_FILE`.
  Ambos archivos deben ser privados y quedar fuera de Git.
- Puertos libres: 18482 en el equipo; 18081 y 18443 dentro de Android.

La CA del Headscale se añade solo al proceso instrumentado. No se modifica el
almacén de confianza del dispositivo ni se desactiva TLS. Evita `adb reverse`
para llegar al servidor: un cambio de VPN puede reiniciar esa conexión.

```bash
mkdir -p builds/android-lab
CGO_ENABLED=0 go -C src build -o ../builds/android-lab/peer ./tools/android-lab-peer

python3 tools/test_android_lab.py --functional \
  --serial "$ANDROID_SERIAL" \
  --headscale-url "$HEADSCALE_LAB_URL" \
  --peer-control-url "$HEADSCALE_LAB_PEER_URL" \
  --control-ca-file "$HEADSCALE_LAB_CA_FILE" \
  --api-key-file "$HEADSCALE_LAB_API_FILE" \
  --key-file "$HEADSCALE_LAB_KEY_FILE" \
  --peer builds/android-lab/peer \
  --peer-data builds/android-lab/estado-peer \
  --log-dir builds/android-lab/registros
```

La prueba configura Headscale por HTTPS, prueba la clave API y el CRUD de
servicios, verifica la CA activa, exporta archivos, configura el cifrado y
programación de copias, crea copias SQLite parcial y completa e importa un
archivo elegido. Después elimina un servicio, cambia ajustes y restaura
realmente la copia. Comprueba el catálogo, la identidad y la CA recuperadas.
Finalmente el segundo nodo abre el servicio HTTPS, validando cadena y nombre.
Se prueban también los rechazos de URL y contraseña incorrectas.

Los archivos cifrados `android.hgw`, `android-selected.hgw` y `android.hgwcfg`
quedan en el directorio de resultados. El código de pruebas utiliza la
contraseña `contraseña-laboratorio-2026`, exclusivamente para esos datos.
Puedes comprobarlos con el controlador de SQLite de Linux:

```bash
CGO_ENABLED=0 go -C src run ./tools/android-lab-interop \
  --archive ../builds/android-lab/registros/android.hgw \
  --profile ../builds/android-lab/registros/android.hgwcfg \
  --password-file "$PASSWORD_LAB_FILE" \
  --headscale-key-file "$HEADSCALE_LAB_API_FILE"
```

Repite con `android-selected.hgw` para la copia parcial. La comprobación lee y
descifra la configuración y los secretos sin imprimir sus valores.

## Túnel del teléfono, opcional

Para añadir TCP, UDP y MagicDNS del teléfono, prepara la sonda y añade `--vpn`
al ejecutor. Es necesario consentir previamente el uso de `VpnService` en la
APK de laboratorio. Si hay otra VPN, acuerda el cambio temporal y restáurala
al terminar. La prueba básica de servicios no sustituye ninguna VPN.

```bash
# Waydroid x86_64; en arm64 utiliza aarch64-linux-android24-clang.
"$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android24-clang" \
  src/tools/android-lab/udp-probe.c -o builds/android-lab/udp-probe
adb -s "$ANDROID_SERIAL" push builds/android-lab/udp-probe /data/local/tmp/hgw-udp-probe
adb -s "$ANDROID_SERIAL" shell chmod 700 /data/local/tmp/hgw-udp-probe
```

Se usa el UID de `adb shell` para comprobar el TUN: el propio UID del Gateway
queda excluido. Se distingue entre consulta directa a MagicDNS y resolución del
sistema; el DNS privado estricto de un proveedor externo puede impedir esta
última. El ejecutor lo informa como pendiente y conserva ese ajuste.

## Comprobación básica y limpieza

Para comprobar solo arranque, sesión y navegación, sin preparar Headscale:

```bash
adb -s "$ANDROID_SERIAL" shell am instrument -w \
  org.headscalegateway.android.lab.test/org.headscalegateway.android.SmokeInstrumentation
```

Al terminar, detén la APK `.lab`, retira sus dos paquetes y elimina servidor,
claves y datos temporales. Restaura la VPN y los ajustes originales de Waydroid.
El ejecutor cierra su nodo auxiliar y elimina el resultado remoto; no administra
el ciclo de vida del servidor. No borres los datos de la APK habitual.

El estado y las validaciones pendientes están en [docs/ANDROID.md](../docs/ANDROID.md).
