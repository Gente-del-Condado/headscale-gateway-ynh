# Instalación privada desde Forgejo con git pull

El código y las carpetas instalables están en la misma rama `main`. No hacen
falta ZIP, repositorios públicos ni ramas para cada plataforma.

| Carpeta del clon | Uso |
| --- | --- |
| `addons/headscale_gateway/` | Copiar a la carpeta local de aplicaciones de Home Assistant |
| `custom_components/headscale_gateway/` | Copiar a `config/custom_components/` para las entidades |
| `yunohost/` | Instalar o actualizar mediante YunoHost desde una carpeta local |
| `packaging/` | Fuentes de los instaladores; no se instalan directamente |

`addons/` y `yunohost/` se generan del mismo código y se incluyen en Git.
Home Assistant compila el contenedor al instalar o reconstruir; no necesitas Go
en su terminal. YunoHost incluye ejecutables Linux amd64/arm64 verificados,
aproximadamente 113 MiB en total sin compresión. La descarga inicial por Git es
mayor por este motivo; el servidor YunoHost tampoco necesita Go.

## Acceso privado

En cada equipo configura acceso SSH a Forgejo antes de clonar. Es preferible una
clave de despliegue de solo lectura para cada equipo, registrada en el repositorio.
No incluyas claves privadas en Git. La clave para descargar código es independiente
de la identidad VPN del Gateway.

Las direcciones SSH de esta guía son ficticias. Sustituye
`ssh://git@forgejo.example.net:2222/usuario/headscale-gateway.git` por la URL
SSH que muestra tu repositorio: incluye el usuario SSH, el servidor, el puerto
y el propietario del repositorio. No utilices contraseñas ni tokens en la URL.

## Home Assistant OS: primera instalación

### Preparar o reutilizar la clave SSH

En la terminal de Home Assistant, con Git y OpenSSH disponibles:

```bash
mkdir -p /data/.ssh
chmod 700 /data/.ssh

if [ ! -f /data/.ssh/headscale_gateway_forgejo ]; then
  ssh-keygen -t ed25519 -a 100 \
    -f /data/.ssh/headscale_gateway_forgejo \
    -C "homeassistant-headscale-gateway"
fi

cp /data/.ssh/headscale_gateway_forgejo.pub /config/headscale_gateway_clave_publica.txt
```

Abre `headscale_gateway_clave_publica.txt` desde File Editor y copia la línea
completa que empieza por `ssh-ed25519`. En Forgejo, entra en **Configuración del
repositorio → Claves de despliegue → Añadir clave** y regístrala con permiso de
solo lectura. Si la clave ya estaba autorizada, reutilízala.

El TXT contiene la clave pública. El archivo sin `.pub` es privado y debe
permanecer en Home Assistant. Si has protegido la clave con una frase de
contraseña, SSH la solicitará al conectar. En la primera conexión verifica
también la huella del servidor antes de aceptarla.

### Clonar e instalar el complemento

Si ya existe el clon, utiliza el procedimiento de actualización siguiente.
Para una primera instalación:

```bash
GIT_SSH_COMMAND='ssh -i /data/.ssh/headscale_gateway_forgejo -o IdentitiesOnly=yes' \
git clone ssh://git@forgejo.example.net:2222/usuario/headscale-gateway.git /data/headscale-gateway-src &&

git -C /data/headscale-gateway-src config core.sshCommand \
  'ssh -i /data/.ssh/headscale_gateway_forgejo -o IdentitiesOnly=yes' &&
mkdir -p /local_apps/headscale_gateway &&
cp -a /data/headscale-gateway-src/addons/headscale_gateway/. /local_apps/headscale_gateway/ &&
head -n 3 /local_apps/headscale_gateway/config.yaml
```

`/local_apps` es la carpeta de aplicaciones locales utilizada en esta guía. Si tu terminal expone
las aplicaciones locales en `/addons`, sustituye `/local_apps` por `/addons`.
No copies estos archivos dentro de `/data/gateway/` del complemento.

En la tienda busca actualizaciones e instala **Headscale Gateway** desde
**Aplicaciones locales**. Arranca y abre su interfaz. El nombre interno de la
aplicación es `headscale_gateway`.

Para entrar desde el móvil o PC: **Ajustes → Aplicaciones/Complementos →
Headscale Gateway → Abrir interfaz web**. Activa **Mostrar en la barra lateral**
en la ficha para tener acceso directo en el menú. Usa una cuenta administradora
de Home Assistant. El servidor no necesita un navegador y no tienes que abrir
el puerto interno 62000–65500 ni configurar antes VPN, DNS o certificados.
Entrarás directamente con tu sesión de administrador de Home Assistant, sin
otra contraseña. La identidad y el grupo `system-admin` deben ser enviados por
Supervisor en cada petición. Si no llegan, se deniega el acceso: actualiza
Home Assistant/Supervisor. El cierre de sesión y MFA se gestionan en Home Assistant.
Las opciones `admin_username` y `admin_password` son opcionales y solo inicializan
una cuenta separada para acceso directo; las cuentas existentes se conservan.

## Home Assistant OS: actualizar desde el clon

```bash
git -C /data/headscale-gateway-src config core.sshCommand \
  'ssh -i /data/.ssh/headscale_gateway_forgejo -o IdentitiesOnly=yes' &&
git -C /data/headscale-gateway-src pull --ff-only &&
if [ -f /local_apps/headscale_gateway/config.yaml ]; then
  cp -a /data/headscale-gateway-src/addons/headscale_gateway/. /local_apps/headscale_gateway/ &&
  head -n 3 /local_apps/headscale_gateway/config.yaml &&
  echo "Código actualizado. Reconstruye Headscale Gateway desde Home Assistant."
else
  echo "No encuentro la instalación anterior. No se ha copiado nada."
fi
```

Después pulsa **Reconstruir** en la aplicación local; copiar archivos no reemplaza
el contenedor en ejecución. Conserva antes una copia de seguridad. No desinstales
para actualizar. Los datos persistentes siguen en `/data/gateway/` dentro de su
contenedor y no se copian desde Git.

## Integración de entidades: instalación y actualización

Este paso es opcional y distinto de ejecutar el complemento. Desde Home
Assistant 2026.11, Marketplace sustituye a HACS, pero ambos requieren repositorios
públicos de GitHub. Para Forgejo privado, copia la integración desde el clon:

```bash
git -C /data/headscale-gateway-src pull --ff-only &&
mkdir -p /config/custom_components/headscale_gateway &&
cp -a /data/headscale-gateway-src/custom_components/headscale_gateway/. /config/custom_components/headscale_gateway/

cat /config/custom_components/headscale_gateway/manifest.json
```

Reinicia Home Assistant Core y añade **Headscale Gateway** desde Dispositivos y
servicios. Usa una URL administrativa alcanzable desde Core y un token del Gateway
`viewer` con alcance `read`. El enlace Ingress del navegador no sirve como URL de
API para la integración.

## YunoHost: primera instalación

En una sesión SSH de tu usuario administrador de YunoHost:

```bash
git clone ssh://git@forgejo.example.net:2222/usuario/headscale-gateway.git "$HOME/headscale-gateway-src"

sudo yunohost app install "$HOME/headscale-gateway-src/yunohost"
```

YunoHost solicita dominio, ruta y permisos. Instala
desde el clon local: su proceso Git no necesita autenticarse contra Forgejo.
No instales `packaging/yunohost/`, que contiene solo las fuentes del paquete.

Abre la aplicación desde el portal con tu cuenta del grupo `admins`: entrarás
sin otra contraseña del Gateway. El grupo se comprueba en cada petición mediante
NSS. Las cuentas existentes se conservan para el acceso directo. El cierre de
sesión y MFA pertenecen al portal de YunoHost. La instalación y actualización
comprueban SSOwat 12.1.1+ y la disponibilidad del grupo `admins`.

## YunoHost: actualizar desde el clon

```bash
git -C "$HOME/headscale-gateway-src" pull --ff-only &&
sudo yunohost app upgrade headscale_gateway --file "$HOME/headscale-gateway-src/yunohost"
```

Para una segunda instancia sustituye `headscale_gateway` por su ID real, por
ejemplo `headscale_gateway__2`. Para probar cambios del paquete sin cambiar su
versión, añade `--force` al comando de actualización. Conserva la copia de
seguridad automática de YunoHost. No reemplaces a mano los ejecutables: el script
gestiona servicio, nginx, permisos y configuración. Los datos siguen fuera del clon.

## Preparar una nueva entrega desde el PC de desarrollo

Después de modificar el programa o las plantillas:

```bash
# Solo para una versión nueva:
# python3 tools/set_version.py 1.4.2

python3 tools/prepare_git_install.py
python3 tools/prepare_git_install.py --check
```

Requiere Go y Python 3.11+, compila ambos binarios Linux y prepara el contexto
completo de Home Assistant sin ZIP. No modifica la aplicación en ejecución,
`bin/`, `releases/`, SQLite, credenciales ni identidades de los equipos.
Rechaza sobrescribir cambios manuales en las carpetas generadas.

Revisa los cambios y súbelos al mismo repositorio:

```bash
git add .
git commit -m "Actualizar código y paquetes instalables"
git push origin main
```

Incluye `addons/`, `yunohost/` y `git-install-manifest.json` en el commit. Después
los equipos solo necesitan `git pull` y los pasos de copia/actualización anteriores.
La verificación comprueba huellas del código, plantillas y archivos instalables;
detecta paquetes antiguos, incompletos o modificados.

Sigue pendiente la aceptación en instalaciones reales de Home Assistant OS y
YunoHost, incluida la restauración. Las pruebas locales no equivalen a esa validación.

Referencias: [aplicaciones locales de Home Assistant](https://contributors.home-assistant.io/apps/first-app/),
[paquetes locales de YunoHost](https://doc.yunohost.org/dev/packaging/test/) y
[estructura y orígenes admitidos por Marketplace](https://contributors.home-assistant.io/marketplace/reference/repository-structure/).
