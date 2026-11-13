# Ejemplos completos

Estos ejemplos utilizan nombres y direcciones ficticios. Cada backend debe poder alcanzarse desde el proceso Gateway, y cada cliente debe resolver el nombre publicado hacia el listener correspondiente.

## Publicar una aplicación de la LAN

Supongamos un Gateway llamado `casa`, una zona `intra.example.net` y una aplicación que escucha en `http://192.0.2.20:8080`.

| Campo en Servicios | Valor del ejemplo |
| --- | --- |
| Nombre | `documentos` |
| Backend HTTP(S) | `http://192.0.2.20:8080` |
| Descripción | `Documentación de la red doméstica` |
| Habilitado | Sí |
| Salida | La ruta que permita alcanzar esa dirección LAN |

La URL del servicio será `https://documentos.casa.intra.example.net/`. Para acceder por LAN al puerto 8443, añade ese puerto a la URL y resuelve el nombre hacia la IP LAN del Gateway. Para el acceso por VPN, utiliza la IP del nodo tsnet y el listener virtual 443.

### Comprobar cada tramo

Primero prueba el backend desde el equipo del Gateway.

```bash
curl --fail --show-error http://192.0.2.20:8080/
```

Después prueba el servicio publicado desde un cliente autorizado, con el certificado público de la CA si empleas CA local.

```bash
curl --fail --show-error \
  --cacert ca-gateway.pem \
  https://documentos.casa.intra.example.net/
```

Si el backend responde pero la URL publicada falla, comprueba por separado DNS, conectividad al puerto, ACL y confianza TLS. Una resolución correcta de DNS solo confirma la dirección.

## Publicar Home Assistant

El acceso administrativo al complemento se abre mediante Ingress. Si además quieres que el Gateway publique el propio Home Assistant como servicio, crea un servicio cuyo backend sea la dirección alcanzable desde el Gateway.

En el complemento con red del anfitrión, un destino habitual es `http://127.0.0.1:8123`. Compruébalo en tu equipo. Home Assistant necesita aceptar las cabeceras del proxy desde su dirección real.

```yaml
# Fragmento de configuration.yaml de Home Assistant.
# Integra estas claves en el bloque http existente.
http:
  use_x_forwarded_for: true
  trusted_proxies:
    - 127.0.0.1  # Solo si esta es la IP de origen que Core observa.
```

Sustituye esa IP por la que observe Core si el Gateway está en otro contenedor o equipo. Tras reiniciar Home Assistant cuando corresponda, abre el servicio publicado, inicia sesión y comprueba que el panel mantiene su conexión WebSocket. El endpoint utilizado por Home Assistant es `/api/websocket`.

## Consultar el estado con un token delegado

Crea un token de lectura en **Sistema → API / Permisos**. Utiliza una URL administrativa HTTPS alcanzable por el cliente. Para consultar también inventario de red necesitas el alcance `network` además de `read`.

En Bash, lee la credencial sin escribirla literalmente en el historial.

```bash
read -rsp 'Token delegado: ' HGW_TOKEN
printf '\n'
export HGW_TOKEN
curl --fail-with-body --show-error \
  --cacert ca-gateway.pem \
  -H "Authorization: Bearer $HGW_TOKEN" \
  https://admin.casa.intra.example.net/api/v1/status
unset HGW_TOKEN
```

Si el certificado ya es de confianza del sistema, puedes omitir `--cacert`. En PowerShell, guarda el token en una variable de entorno de la sesión y consulta la API.

```powershell
$token = Read-Host 'Token delegado' -AsSecureString
$env:HGW_TOKEN = [System.Net.NetworkCredential]::new('', $token).Password
Invoke-RestMethod -Uri 'https://admin.casa.intra.example.net/api/v1/status' `
  -Headers @{ Authorization = "Bearer $env:HGW_TOKEN" }
Remove-Item Env:HGW_TOKEN
```

Las operaciones de copias, identidad y otros módulos administrativos pueden requerir sesión interactiva. Consulta los permisos de cada ruta en **Sistema → API / CLI**. Los tokens y sus roles no sustituyen esos requisitos.

## Exportar un perfil o una copia

| Necesidad | Formato | Qué revisar |
| --- | --- | --- |
| Reutilizar ajustes en otro nodo | Perfil `.hgwcfg` | Verifica los campos seleccionados y la configuración del nodo receptor. |
| Recuperar una instalación | Copia cifrada `.hgw` | Conserva la contraseña y comprueba los componentes incluidos. |
| Copiar una política de Headscale | Exportación ACL | Revisa la política antes de aplicarla en otro servidor. |

Una copia completa puede incluir la identidad VPN. Durante una migración, detén el nodo original antes de poner en marcha su identidad restaurada. El [manual de uso](USO_COMPLETO.md#copias-actualización-y-migración) detalla las diferencias entre actualización, migración y alta de otro nodo.

## Ejemplos descargables

El directorio [startup.json](../examples/startup.json) y [acl-templates.json](../examples/acl-templates.json) contiene los ejemplos de formato que acompañan al código. Revisa los identificadores y los campos del esquema de tu versión antes de importarlos. El índice API del panel se genera desde las rutas registradas por el motor.
