# Solicitud para F-Droid

Esta carpeta prepara la receta; todavía no representa aceptación en F-Droid.
Los fuentes completos deben publicarse en la rama `upstream-source` del
repositorio público indicado en los metadatos, con la etiqueta `source-v1.4.7`.
No basta con el manifiesto y los binarios del paquete YunoHost de `main`.

El script compila Go desde los fuentes comunes; no descarga APK, AAR ni motores
precompilados del proyecto. El toolchain Go oficial se verifica por SHA-256.
Gradle construye JNI desde los fuentes HEV incluidos con sus licencias. La
variante F-Droid elimina la firma del titular. El wrapper Gradle conserva los
avisos y una suma de distribución fija. Los fuentes Go se descargan según
`go.mod`/`go.sum`, sin cambiar versiones.

La receta 1.4.7 pasa localmente `fdroid readmeta` y `fdroid lint` con
fdroidserver 2.4.5 y las categorías oficiales. Antes de enviar la receta, ejecuta en el entorno oficial de fdroidserver:

```sh
fdroid readmeta
fdroid lint org.headscalegateway.android
fdroid build --server org.headscalegateway.android:10040074
```

La compilación local de la variante sin firma no equivale a esta validación del
servidor oficial. Revisa los resultados del escáner, sin ignorar carpetas enteras
ni añadir excepciones genéricas para binarios. Los módulos Go pueden incorporar
recursos generados por terceros; conserva sus licencias y resuelve cualquier
objeción del revisor aportando los fuentes o desactivando el recurso afectado.

La ficha localizada está en `android/fastlane/metadata/android/`. Faltan capturas
de un teléfono físico y la revisión de inclusión. Envía la receta únicamente
cuando el repositorio y la etiqueta públicos existan.

Referencias: [inclusión](https://f-droid.org/docs/Inclusion_Policy/),
[metadatos](https://f-droid.org/docs/Build_Metadata_Reference/) y
[solicitud](https://f-droid.org/docs/Submitting_to_F-Droid_Quick_Start_Guide/).
