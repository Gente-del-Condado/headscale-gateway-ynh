# Fuentes de compilación de Headscale Gateway

Este archivo acompaña las fuentes del motor incluidas en el mismo paquete que
sus binarios. `VERSION` identifica la versión; `build-manifest.json` del paquete
registra las sumas de las fuentes y de los binarios YunoHost. Las dependencias
están fijadas en `go.mod` y `go.sum`; sus fuentes y avisos se incluyen en `vendor/`.

Licencia del código propio: **AGPL-3.0-only**, con el alcance de `NOTICE` y los
términos de `LICENSE`. Los componentes de terceros conservan sus licencias;
sus avisos acompañan al paquete en `notices/third_party/`.

Extrae `source.tar.gz` en una carpeta vacía. Desde esa carpeta, con Go 1.27.4:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -buildvcs=false -trimpath -o headscale-gateway-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -mod=vendor -buildvcs=false -trimpath -o headscale-gateway-linux-arm64 .
```

Las fuentes corresponden a las dependencias usadas por el generador YunoHost.
`-mod=vendor` permite compilar sin descargar módulos; la distribución oficial
usa `-mod=readonly` sobre los mismos módulos fijados. El complemento Home Assistant usa el
mismo motor desde su Dockerfile y elige la arquitectura de su contenedor.
Estas compilaciones normales no activan el módulo opcional `commercial`.

Fuentes públicas del motor:
https://github.com/Gente-del-Condado/headscale-gateway_ynh/blob/main/payload/source.tar.gz

Conserva las fuentes correspondientes de cada entrega que redistribuyas. Un
enlace a una rama cambiante no sustituye los fuentes de esa entrega concreta.
Si modificas el programa y lo ofreces por red, aplica también la sección 13 de
AGPL-3.0: ofrece a sus usuarios los fuentes correspondientes de tu versión.
