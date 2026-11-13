# Recuperar el acceso y comprobar el bloque de consola

La recuperación rota la credencial de emergencia. **No cambia por sí sola la contraseña de una cuenta**. Se realiza en el equipo del Gateway, con el mismo usuario y la misma ruta `--db`. No borres SQLite ni `master.key`.

## Linux: paquete del servidor

Detén el proceso anterior con Ctrl+C. Abre una terminal en la carpeta que contiene el ejecutable:

```bash
./headscale-gateway --db "$HOME/.local/share/headscale-gateway/gateway.sqlite" --reset-recovery
./headscale-gateway --db "$HOME/.local/share/headscale-gateway/gateway.sqlite"
```

En el proyecto completo, sustituye `./headscale-gateway` por `./bin/linux/headscale-gateway` (amd64) o `./bin/linux-arm64/headscale-gateway` (arm64).

## Windows 11: paquete del servidor

Extrae primero el ZIP; no ejecutes dentro de la vista del archivo comprimido. Detén la instancia anterior con Ctrl+C. Abre PowerShell en la carpeta donde está `headscale-gateway.exe`:

```powershell
.\headscale-gateway.exe --db "$env:LOCALAPPDATA\HeadscaleGateway\gateway.sqlite" --reset-recovery
.\headscale-gateway.exe --db "$env:LOCALAPPDATA\HeadscaleGateway\gateway.sqlite"
```

En el proyecto completo x64, utiliza `.\bin\headscale-gateway.exe`. Si tu instalación está en `%ProgramData%` u otra ubicación, conserva **esa ruta**, en lugar de crear accidentalmente una instalación nueva en LocalAppData.

## Qué debe aparecer

1. El comando con `--reset-recovery` confirma la rotación y termina.
2. El segundo comando arranca el servidor. Después del arranque de VPN, o a los 20 segundos de abrir administración, aparece un bloque `HEADSCALE GATEWAY · ACCESO AL PANEL`. En configuración inicial aparece inmediatamente.
3. Dentro aparecen la URL local y `CÓDIGO TEMPORAL DE RECUPERACIÓN`, seguido del código resaltado. Caduca 15 minutos después de mostrarse y solo se puede canjear una vez.
4. Abre `http://127.0.0.1:9091/admin/` desde ese ordenador. Algunas terminales permiten clic o Ctrl+clic; si no, copia la URL. Si cambiaste el puerto administrativo, utiliza la URL real del bloque.
5. Abre la recuperación local e introduce el código. El panel entrega la credencial de recuperación; sigue sus indicaciones para acceder. Configura después tu cuenta en Acceso / Identidad y comprueba que puedes entrar con ella.

No incluyas el código en capturas públicas ni lo pegues en la URL. No redirijas stderr (`2>`, `2>&1`) durante esta comprobación. Una consola sin capacidad de color muestra el mismo bloque en texto. `NO_COLOR` desactiva el color; `GATEWAY_LANG=en` permite comprobar la versión inglesa.

## Linux instalado como servicio systemd

El servicio no tiene una terminal interactiva: el código **no se escribe en journalctl**. Para una instalación con usuario `gateway` y datos en `/var/lib/headscale-gateway`:

```bash
sudo systemctl stop headscale-gateway
sudo -u gateway /usr/local/bin/headscale-gateway --db /var/lib/headscale-gateway/gateway.sqlite --reset-recovery
sudo -u gateway /usr/local/bin/headscale-gateway --db /var/lib/headscale-gateway/gateway.sqlite
```

Completa el acceso local. Después pulsa Ctrl+C y ejecuta `sudo systemctl start headscale-gateway`. Adapta usuario y ruta si tu servicio usa otros. En Windows con un servicio configurado por terceros, detén ese servicio y ejecuta interactivamente bajo la misma identidad; el proyecto no instala un servicio Windows automáticamente.

## Si no aparece el código

- Revisa que has actualizado **el ejecutable que realmente arrancas**.
- Confirma que `--reset-recovery` y el arranque utilizan la misma SQLite.
- Un arranque normal de una instalación ya configurada muestra instrucciones, no genera otro código.
- Si la salida está redirigida, la recuperación queda pendiente para el siguiente arranque interactivo.
- Si aparece `address already in use`, detén la otra instancia; cambiar el puerto no resuelve dos procesos contra la misma SQLite.
- En SSH, el navegador debe llegar al loopback del servidor mediante un túnel, no al loopback del equipo cliente por error.
