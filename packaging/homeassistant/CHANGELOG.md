# 1.4.7

- Versión común del motor, complemento, paquete YunoHost y clientes de escritorio y Android.
- Documentación de instalación, privacidad, firmas y canales actualizada en ambos manuales públicos.
- Preparación del catálogo YunoHost y publicación de los fuentes completos sin historial privado.
- Se conservan los controles de seguridad previos a la publicación automática en Mis-app.

# 1.4.6

- Actualización de Caddy a 2.11.5, go-jose a 4.1.6 y QUIC a 0.63.0 para corregir las alertas de seguridad de sus dependencias.
- Actualización de glob y minimatch en las herramientas de empaquetado de la bandeja.
- Compilación con Go 1.27.4 y actualización de x/net y compress para incorporar correcciones de HTTP, TLS y compresión.
- Renovación del inventario y los avisos de licencia de las dependencias actualizadas.

# 1.4.5

- Envío y recuperación manual de perfiles y copias cifradas en repositorios privados de Forgejo.
- Icono y logotipo del complemento en la tienda de Home Assistant.
- Publicación automática del complemento en Mis-app a partir de la rama principal.

# 1.4.4

- Guías de instalación independientes de otros proyectos.
- Direcciones SSH ficticias en los ejemplos, con instrucciones para sustituirlas por las del repositorio propio.

# 1.4.3

- Corrección del enlace «Abrir interfaz web»: la ruta de entrada es relativa a Ingress.
- Se evita la doble barra que redirigía a `/admin/` fuera de Home Assistant y podía producir un 404.
- La prueba de contenedor construye el enlace desde el manifiesto y comprueba el recorrido por un proxy simulado.

# 1.4.2

- Corrección del arranque: la API de Supervisor no devuelve el campo `panel_admin`.
- La opción `ingress_panel` solo controla la barra lateral y no bloquea el arranque si está desactivada.
- Se conserva la verificación del administrador, del proxy y de CSRF en cada petición.
- Pruebas actualizadas para reproducir la respuesta real de Supervisor.

# 1.4.1

- Documentación de instalación, uso, actualización y recuperación en español.
- Guías de Home Assistant, YunoHost e integración de entidades unificadas con el repositorio privado.
- Descripción predeterminada del complemento en español, sin cambios funcionales.

# 1.4.0

- Motor común de sesiones delegadas para Home Assistant y el portal de YunoHost.
- Prueba privada del proxy obligatoria al delegar autenticación por loopback.
- Se conservan la sesión administradora de Ingress y la protección CSRF.

# 1.3.0

- Acceso directo desde Ingress con la sesión administradora de Home Assistant.
- Verificación del proxy, del identificador de usuario y del grupo administrador en cada petición.
- Protección CSRF sin crear una cookie de acceso reutilizable fuera de Ingress.
- Cuenta inicial opcional reservada al acceso directo.

# 1.2.1

- Instrucciones de acceso al panel y a la barra lateral al arrancar en Home Assistant.
- El puerto interno reservado al proxy deja de anunciarse como enlace para el navegador.
- Comprobación del panel incrustado, los recursos, la cuenta inicial y las sesiones mediante Ingress.

# 1.2.0

- Un único servidor HTTP, registro de rutas, contrato de arranque y autenticación.
- Inicialización compartida por los instaladores, sin seleccionar una variante de la aplicación.
- Versión común y estructura estándar de integración HACS en el repositorio principal.
- Conservación de la identidad VPN en las instalaciones y migraciones.

# 1.1.0

- Primer empaquetado experimental para Home Assistant OS.
