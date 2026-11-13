# CA Manager y federación de confianza (primera entrega)

## Alcance actual

Esta entrega conserva sin cambios el primer inicio y la creación de la CA
local. Añade una pestaña **CA y confianza**, un inventario de certificados
públicos CA, alta pendiente de autoridades remotas, aprobación por huella
SHA-256, revocación de la aprobación, descarga pública, comprobación remota
por HTTPS, instalación/desinstalación en Linux y Windows cuando el proceso
tiene privilegios, y descubrimiento opcional de candidatos.

La **CA privada** y la clave `ca.key` no se exportan ni comparten. El fichero
`data_dir/local-ca/<dominio>/ca.crt` sigue siendo la raíz de la CA actual.
Los peers públicos aprobados se almacenan en `data_dir/pki/trust/` con
permisos restrictivos. Una copia TODO cifrada de `data_dir` los incluye.

## Emparejamiento paso a paso

1. Activa «Publicar certificado público» en el Gateway A. El recurso
   `/pki/v1/public-ca` entrega únicamente el certificado PEM público.
2. Obtén el `.crt` por un canal seguro y compara la huella SHA-256 con el
   administrador del Gateway A por un canal independiente.
3. En Gateway B, importa el `.crt`, asigna un nombre y una URL HTTPS base
   opcional (por ejemplo `https://nas.intra.example.net`).
4. **No se confía todavía** en la nueva CA. En B, pulsa «Verificar y aprobar»
   e introduce manualmente la huella completa. Realiza el procedimiento
   análogo en A si deseas confianza bidireccional.
5. Opcionalmente pulsa «Instalar en SO». Aceptar el peer en el inventario
   **no** cambia el almacén global del sistema.
6. «Sincronizar» consulta al otro Gateway mediante HTTPS validado y comprueba
   que el certificado CA ofrecido siga teniendo exactamente la misma huella.
   Una nueva raíz exige una nueva aprobación; nunca se confía automáticamente
   en un cambio de clave o raíz.

La comprobación se repite cada seis horas mientras el Gateway está en marcha
para los peers autorizados que tengan URL. Se actualiza su última fecha de
verificación. No hay replicación de claves privadas ni instalación de CA
como consecuencia de la sincronización.

## Descubrimiento

Los puertos pueden personalizarse. El intercambio inicial de CA pública por HTTP LAN es opcional y queda siempre pendiente de aprobación; consulta [RED_Y_CA_LAN.md](RED_Y_CA_LAN.md).

El descubrimiento combina candidatos de Headscale, DNS y anuncios LAN UDP en `239.255.42.91:47991`; puede complementarse con DNS-SD/Avahi donde esté disponible. Los anuncios son pistas no autenticadas. No hay confianza automática ni un escaneo de todos los servicios de la zona DNS.

En CA y confianza, «Descubrir y descargar CA pendientes» intenta primero Tailscale HTTPS 443 mediante la identidad tsnet del Gateway; después las direcciones LAN conocidas, normalmente 8443. Para cada destino intenta `https://admin.<nodo>/pki/v1/public-ca` y después el dominio principal. La descarga de bootstrap puede aceptar una raíz aún desconocida, pero exige hostname y fechas correctos y la guarda **sin aprobar**. Compara la huella por otro canal antes de aprobar.

El catálogo de servicios solo se consulta mediante HTTPS validado, usando CA aprobadas y/o el almacén del sistema. Una descarga de CA pendiente no autoriza catálogos, instalación en SO ni administración remota. El procedimiento entre Windows y Linux está en [FEDERACION_WINDOWS.txt](FEDERACION_WINDOWS.txt).

## Almacenes de confianza del sistema operativo

La instalación de una CA es una **operación de alta confianza**: podría
permitir que esa autoridad expidiera certificados TLS aceptados por numerosas
aplicaciones. Nunca instales una CA sin reconocer y verificar al titular.

Pulsa «Descargar CA pública» y verifica el SHA-256 antes de instalarla. El
botón del panel intenta instalarla sin elevación automática; en la mayoría
de instalaciones Gateway funciona como usuario sin privilegios y, por tanto,
dará el mensaje de permisos necesarios.

### Fedora / RHEL

```bash
# Reemplaza HUELLASHA256 y ./ca-peer.crt por los valores reales.
sudo install -m 0644 ./ca-peer.crt /etc/pki/ca-trust/source/anchors/hgw-HUELLASHA256.crt
sudo update-ca-trust extract
```

Desinstalación:

```bash
sudo rm /etc/pki/ca-trust/source/anchors/hgw-HUELLASHA256.crt
sudo update-ca-trust extract
```

### Debian / Ubuntu

```bash
sudo install -m 0644 ./ca-peer.crt /usr/local/share/ca-certificates/hgw-HUELLASHA256.crt
sudo update-ca-certificates
```

Desinstalación:

```bash
sudo rm /usr/local/share/ca-certificates/hgw-HUELLASHA256.crt
sudo update-ca-certificates --fresh
```

### Windows (PowerShell como administrador)

Comprueba la huella SHA-256 del certificado DER antes de confiar:

```powershell
$cert = [System.Security.Cryptography.X509Certificates.X509Certificate2]::new("C:\ruta\ca-peer.crt")
$hash = [System.Security.Cryptography.SHA256]::Create()
$sha256 = ([BitConverter]::ToString($hash.ComputeHash($cert.RawData))).Replace("-", "").ToLowerInvariant()
$hash.Dispose()
$sha256 # Comprueba con la huella ofrecida por el otro Gateway
Import-Certificate -FilePath "C:\ruta\ca-peer.crt" -CertStoreLocation Cert:\LocalMachine\Root
```

Para retirarla, localiza **exactamente** ese certificado en `certlm.msc` o usa
`Remove-Item Cert:\LocalMachine\Root\<SHA1Thumbprint>` tras verificar
que corresponde a la misma huella SHA-256. El botón automático utiliza
`certutil` y requiere permisos administrativos; no activa UAC silenciosamente.

### macOS, Android y navegadores

Por ahora, importación manual de la CA pública. En macOS se utiliza el
Llavero del sistema con verificación manual de huella. Android puede requerir
instalación a través de Ajustes y muchas aplicaciones **no confían** en las
CA instaladas por el usuario. Firefox, perfiles aislados y contenedores
pueden usar almacenes propios. Instalar una CA solo en Fedora no resuelve
el aviso HTTPS en un teléfono Android.

## Estado y límites actuales

- No se modifica `ca.key`, no se rota la CA propia ni se emiten certificados
  nuevos desde este módulo.
- Todavía no hay editor de perfiles de emisión, CRL ni OCSP, ni autoridad
  intermediaria. La revocación implementada es de **confianza entre peers**,
  no una CRL de certificados TLS previamente emitidos.
- El descubrimiento LAN requiere Avahi; no se anuncia de forma automática.
- La sincronización es de **comprobación de identidad pública y vigencia**,
  no un sistema de emparejamiento mTLS bidireccional con políticas por peer.
- No se instalan certificados en otros dispositivos del tailnet de forma remota.
- En producción, evita ejecutar el Gateway completo como root para conseguir
  que el botón «Instalar» funcione. Prefiere instalación supervisada por SSH
  o un helper dedicado de privilegios mínimos y políticas estrictas.
- Una nueva CA en el almacén del sistema puede necesitar reiniciar procesos
  que ya habían cargado las raíces de confianza.
- Antes de comercializar el módulo, realiza revisión de seguridad, pruebas
  reales entre dos nodos y una estrategia de rotación y recuperación.
