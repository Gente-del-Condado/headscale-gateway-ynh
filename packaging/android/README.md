# Canales de Android

La fuente de todas las variantes es `android/` más el motor Go de `src/`.
La versión se toma de `VERSION`; no hay un motor distinto para cada tienda.

| Canal | Salida | Firma | Publicación |
| --- | --- | --- | --- |
| Descarga directa | APK release | Clave estable del titular | Release del proyecto |
| F-Droid | APK release sin firmar | La asigna F-Droid | Receta y revisión independientes |
| Google Play | AAB release | Clave de subida estable | Solo preparado; requiere consola y revisión |

Consulta [compilación y uso](../../android/README.md),
[privacidad](../../docs/PRIVACIDAD_ANDROID.md), la
[preparación de F-Droid](fdroid/README.md) y la
[preparación de Play](play/PUBLICACION.md).

La firma debe conservarse fuera del repositorio y respaldarse de forma segura.
Cambiar de firma o de tienda puede requerir exportar, desinstalar e importar.
Una compilación correcta no sustituye las pruebas en teléfonos físicos.
