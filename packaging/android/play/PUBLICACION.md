# Preparación de Google Play · 1.4.7

Estado: AAB preparado; no se ha creado una cuenta ni publicado una aplicación.
La revisión de Google y las pruebas exigidas a la cuenta son pasos externos.

- Identificador: `org.headscalegateway.android`.
- Versión: `1.4.7`; versionCode: `10040074`; min SDK 24; target SDK 36.
- Arquitecturas: arm64-v8a y x86_64; bibliotecas alineadas a 16 KiB.
- AAB: `builds/android/headscale-gateway-play.aab`.
- Privacidad pública: https://gente-del-condado.github.io/headscale-gateway_ynh/PRIVACIDAD_ANDROID.html
- Categoría propuesta: Herramientas. Sin anuncios, compras ni suscripción obligatoria.

## Firma y acceso

El AAB utiliza la identidad estable local. Al crear Play App Signing decide si
Google conservará esa clave de aplicación o si utilizarás otra; las firmas de
F-Droid, Play y descarga directa pueden ser distintas. Con firmas distintas no
se puede actualizar una instalación desde otro canal sin desinstalarla. Exporta
antes los datos y conserva la contraseña de la copia. No subas la clave privada
al repositorio ni la envíes por chat. La consola necesita la clave pública de
subida y, según la opción elegida, un procedimiento separado de inscripción.

Para revisión se debe facilitar un Headscale temporal y datos de prueba, nunca
las credenciales de producción. Documenta el alta del nodo, la apertura del
panel y cómo conectar/desconectar las aplicaciones. Prepara un vídeo de esas
operaciones y capturas reales del móvil antes de enviar la solicitud.

## Declaraciones que debe revisar el titular

`VpnService` es una función principal: acceso de las aplicaciones del teléfono
a una red Headscale privada autogestionada. El tráfico privado se cifra entre
nodos; un nodo de salida es opcional. Se presenta una explicación y una acción
afirmativa antes del permiso Android. No hay monetización ni publicidad mediante
el tráfico. Documenta expresamente esta función en la ficha.

Los dos servicios en primer plano usan `specialUse`: el nodo publica servicios
HTTPS durante una sesión iniciada por el usuario; el otro mantiene el túnel
opcional de aplicaciones. Tienen notificación persistente y controles de
parada. Aporta la explicación y demostración que solicite la consola.

Seguridad de los datos: no hay analítica ni recolección por un servidor del
desarrollador. Hay tratamiento local y transferencias funcionales a servidores
seleccionados por el usuario (IP, identidad de nodo, DNS, configuración y copias
si las activa). Revisa las definiciones y excepciones actuales del formulario;
no marques automáticamente «no se recopilan datos» sin comprobar el alcance de
esos destinos y las opciones que se entregan al revisor. La aplicación permite
borrar los datos locales y gestionar las copias externas por separado.

Completa clasificación de contenido, audiencia, datos de contacto, país,
identidad del titular y pruebas cerradas si corresponden a la cuenta. No se
promete aceptación ni se han rellenado formularios en nombre del titular.

Referencias oficiales: [VPN](https://support.google.com/googleplay/android-developer/answer/12564964),
[firma](https://developer.android.com/studio/publish/app-signing) y
[nivel de API](https://support.google.com/googleplay/android-developer/answer/11926878).
