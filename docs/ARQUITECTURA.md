# Arquitectura del Gateway

## Objetivo

Las unidades de código deben tener un responsable claro y minimizar
operaciones destructivas. Los contratos de SQLite, API, copias y arranque se verifican con pruebas de regresión.

## Un solo núcleo

Todas las instalaciones comparten binario, rutas, SQLite y autenticación.
`startup_options.go` lee el contrato común de arranque; `HTTPOptions` prepara
el mismo `Server.Handler` para acceso directo o mediante proxy. Hay un único
listener administrativo. No existe una selección de plataforma en el núcleo.
Los lanzadores de `packaging/` solo traducen opciones del anfitrión al contrato
común; la integración HACS reside en `custom_components/headscale_gateway/`.

## Flujo de arranque

Los archivos Go de este recorrido pertenecen al módulo `src/`. Los paquetes
compartidos residen en `src/internal/`; sus nombres indican su responsabilidad.

```text
main.go
  ├─ src/internal/cli/           comandos doctor, logs, version (sin VPN)
  ├─ src/internal/bootstrap/     flags y detección de conflictos
  ├─ startup_logging.go     nivel/rotación persistidos
  └─ startup_gateway.go
       ├─ modo configuración: panel local
       ├─ HTTPS LAN: CA local hasta tener nombre estable
       ├─ administración: rutas + sesiones + CSRF
       ├─ vpn_supervisor.go: único dueño de tsnet
       └─ dns_generation.go: publicar cuando se confirme la identidad
```

La recarga dinámica se concentra en `runtime_reload.go`. El runtime prepara
un `gatewaySnapshot` nuevo antes de publicarlo; las peticiones ya iniciadas
continúan con su instantánea anterior. DNS se sincroniza antes del cambio
activo. Un error impide publicar una configuración incompleta.

## Proxy y DNS

`proxy.go` convierte servicios en proxies; `proxy_transport.go` aplica los
límites de conexión y administra los pools de clientes HTTP. **No existe
caché de contenido HTTP**: cerrar conexiones inactivas no borra sesiones ni
archivos del backend.

`dns_records.go` modifica únicamente registros A con la marca de propiedad
correcta. `dns_cleanup.go` limita las eliminaciones al namespace de cada
nodo. `dns_sync.go` construye la publicación de nodos y servicios. Los
adaptadores de Technitium y Cloudflare no deciden qué registros borrar.

## HTTP de administración

- `src/internal/admin/routes_static.go`: whitelist de recursos embebidos e idiomas.
- `src/internal/admin/routes_api.go`: índice de módulos REST.
- `src/internal/admin/middleware_http.go`: cabeceras, idioma y diagnóstico HTTP.
- `src/internal/admin/server.go`: `Controller`, servidor y autorización común.
- `src/internal/admin/routes_*.go`: operaciones específicas por dominio.
- `src/internal/admin/panel-*.js`: vistas funcionales sin frameworks externos.

La lógica HTTP no debe escribir secretos en logs ni aceptar rutas de archivo
arbitrarias enviadas por el navegador. Toda operación que modifique el estado
comprueba autenticación, autorización de módulo y CSRF cuando corresponda.

## Seguridad y límites de confianza

| Componente | Límite |
|---|---|
| Headscale API | Credencial de API remota, operaciones listadas explícitamente |
| tsnet | Identidad del Gateway; no reutilizar automáticamente tailscaled del SO |
| CA local y federada | La CA privada nunca viaja entre Gateways |
| Backups cifrados | Separación entre datos locales y ACL remota |
| Perfiles transportables | No trasladar identidad tsnet ni recovery token |
| Web | Roles admin/operator/viewer y validación de origen |

## Reglas al ampliar el proyecto

1. Mantener modelos públicos y rutas del API al mover archivos.
2. Escribir las funciones de dominio sin dependencias del navegador.
3. Inyectar interfaces para recursos externos (DNSProvider, Controller).
4. Incluir pruebas de los casos de error antes de publicar cambios de red.
5. Mantener soporte para Windows/Linux mediante archivos `*_windows.go`,
   `*_linux.go` o adaptadores propios; no introducir `syscall.Exec` sin alternativa.
6. Documentar las migraciones SQLite antes de modificar las tablas.
7. No activar funciones sensibles por descubrimiento sin aprobación explícita.

## Trabajo todavía necesario

Todavía existen dependencias compartidas en `package main`, así como
componentes de autenticación y PKI avanzados que requieren suites de
integración reales. Separarlos en paquetes nuevos exigirá migraciones
controladas de interfaces y builds completos en Windows, Linux y macOS.
No confundir un análisis de sintaxis con una prueba de funcionamiento.

## Activación comercial opcional

`src/internal/licensing` valida documentos Ed25519 independientes del proveedor de pago. La etiqueta de compilación `commercial` activa el control; sin ella no se crean archivos de licencia ni se restringen servicios. La clave pública se fija al compilar. `src/tools/license-admin` es una herramienta privada del emisor, no un servicio de cobro.

El control se aplica después de despachar administración y CA pública, antes del portal y proxy. Una licencia ausente/caducada no bloquea recuperación ni copias. Los detalles de fechas, gracia, integración y límites están en [SUSCRIPCIONES.md](SUSCRIPCIONES.md).
