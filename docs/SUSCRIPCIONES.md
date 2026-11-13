# Módulo comercial de activación y suscripciones

## Alcance de esta versión

El módulo está **desactivado en las compilaciones normales entregadas**. No cambia el funcionamiento de tu instalación. Para habilitarlo hay que recompilar con la etiqueta Go `commercial` y una clave pública Ed25519 del emisor. No se puede habilitar o deshabilitar mediante una preferencia del cliente.

Es independiente de Stripe, PayPal o cualquier proveedor. Verifica localmente un documento firmado; no contiene claves de cobro, no realiza cargos y no llama a servidores de licencias. La renovación automática y el servicio de cobro se integran por separado. No se incluye todavía un portal de clientes ni un adaptador de pagos desplegado.

Con el módulo activado, una licencia válida para esta instalación permite atender nuevas peticiones del proxy, portal y catálogo. Al finalizar la validez puede haber un periodo de gracia **firmado**, de hasta siete días. Después, esas peticiones reciben HTTP 402. Administración, recuperación, copias, configuración y publicación de CA siguen accesibles según sus permisos habituales. No se destruyen datos, no se detiene tsnet, no se revocan rutas y no se cortan conexiones de proxy ya abiertas.

La mini GUI continúa permitiendo abrir el panel y consultar estado. El resumen administrativo muestra el estado de suscripción, identificador de instalación y vencimiento; `GET /api/v1/status` lo expone bajo `subscription` a quien tenga permiso de lectura.

## Preparación del emisor (solo tú)

Guarda la clave **privada fuera del proyecto, de Git y de las entregas**. El directorio solicitado debe ser nuevo:

```bash
go -C src run ./tools/license-admin keygen --dir "$HOME/.local/share/hgw-issuer"
./build.sh --commercial-public-key "$HOME/.local/share/hgw-issuer/issuer.pub"
```

PowerShell:

```powershell
go -C src run ./tools/license-admin keygen --dir "$env:LOCALAPPDATA\HGW-Issuer"
.\build.ps1 --commercial-public-key "$env:LOCALAPPDATA\HGW-Issuer\issuer.pub"
```

El compilador exige una clave pública de 32 bytes y no acepta una clave privada como parámetro. El emisor genera `issuer.key` (privada) e `issuer.pub` (pública). Restringe los permisos NTFS del directorio privado en Windows; los permisos Unix no equivalen a ACL NTFS.

Para desactivar completamente la restricción, recompila con `./build.sh` o `.\build.ps1` **sin** `--commercial-public-key`. Los destinatarios mantienen su derecho a modificar y recompilar el programa bajo AGPL-3.0; la activación no restringe esos derechos.

## Activación manual de una instalación

1. Ejecuta la compilación comercial. Abre administración local y consulta el resumen: genera `license-instance.id` dentro del directorio de datos. También puedes leer ese archivo; no es una clave secreta. No cambies esa identidad mientras la licencia esté vigente.
2. En el equipo del emisor, prepara un JSON con los campos siguientes. Las fechas son segundos Unix UTC. Deben ser fechas reales: `not_before < expires_at <= grace_until`, con un máximo de siete días de gracia. `features` debe contener `gateway`.

```json
{
  "version": 1,
  "product": "headscale-gateway",
  "license_id": "licencia-unica-del-periodo",
  "customer_id": "referencia-interna-del-cliente",
  "instance_id": "COPIAR_ID_DE_LA_INSTALACION",
  "not_before": 1792886400,
  "expires_at": 1795478400,
  "grace_until": 1795651200,
  "features": ["gateway"]
}
```

Estas fechas son únicamente un ejemplo de formato. Para obtener la hora Unix actual: `date +%s` en Linux o `[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()` en PowerShell.

3. Firma usando un archivo de salida nuevo:

```bash
go -C src run ./tools/license-admin sign --key "$HOME/.local/share/hgw-issuer/issuer.key" --claims cliente.json --out subscription.license
```

4. Entrega **solo** `subscription.license` al cliente. Instálalo en su `data_dir`, junto a `license-instance.id`, mediante sustitución atómica de archivo. En Linux: copiar a un archivo temporal en el mismo directorio, `chmod 600` y `mv` al nombre final. En Windows, detener el proceso durante la sustitución evita lecturas parciales. Nunca distribuyas `issuer.key`.
5. El servidor relee el estado con una caché de hasta cinco segundos. Comprueba `subscription.state` (`active` o `grace`) y una petición al portal. La instalación que use otro identificador no debe aceptarlo.

## Contrato para cualquier plataforma de pago

La arquitectura es:

```text
Proveedor de cobro → webhook verificado → servicio privado de derechos
                                         ↓ firma Ed25519
                                  subscription.license → Gateway
```

El servicio de derechos se desarrolla/aloja fuera del Gateway. Debe validar la firma del proveedor, deduplicar por ID de evento, consultar el estado actual de la suscripción (los eventos pueden llegar desordenados), asociar cliente/instalación y emitir fechas concretas. Un aviso de checkout no basta para asumir que se ha cobrado.

Estados activos o pruebas autorizadas pueden emitir un documento válido hasta la fecha concedida. Una cancelación deja de renovarlo; no envíes órdenes destructivas. Un impago puede usar la gracia firmada definida por tu contrato. El retorno HTTPS para obtener el archivo debe autenticar al cliente y no exponer la clave privada.

La función Go `licensing.Sign(Claims, ed25519.PrivateKey)` y el comando `tools/license-admin sign` son los puntos de integración. El formato transportado es `base64url(JSON).base64url(firma)`, sin padding; se firma el JSON exacto, no una recomposición del receptor. El producto/version/instalación/fechas/features quedan cubiertos por la firma.

Referencia de ejemplo para un adaptador futuro: [eventos de suscripciones Stripe](https://docs.stripe.com/billing/subscriptions/webhooks). No es una dependencia de esta implementación.

## Límites que debes conocer antes de vender

Esta activación no es DRM invulnerable. Quien controla el equipo puede modificar el binario, clonar el directorio de instalación o alterar el reloj. No hay revocación inmediata offline: un documento firmado sigue siendo válido hasta su vencimiento/gracia. Usa concesiones cortas y un servicio de renovación autenticado si necesitas revocación y control de plazas. La rotación de clave pública requiere nuevas compilaciones en esta primera versión.

El módulo opcional se distribuye bajo la misma AGPL-3.0-only que el código propio del proyecto. Cobrar por soporte, alojamiento o suscripciones no retira a los usuarios sus derechos a estudiar, modificar y redistribuir el programa. Deben facilitarse los fuentes correspondientes cuando lo exige la licencia. Nunca publiques la clave privada del emisor, credenciales o datos de clientes. Consulta `LICENCIAS.md` para distinguir la licencia del software de las condiciones del servicio.
