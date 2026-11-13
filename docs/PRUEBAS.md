# Plan de regresión y despliegue seguro

## Automatizadas

```bash
./tools/verify.sh       # Linux/macOS: go -C src test, vet, build, JS y HTML
# o en PowerShell:
.\tools\verify.ps1
```

Comprobación del bloque de recuperación en un proceso aislado Linux/macOS:

```bash
python3 tools/check_console.py
```

Pruebas del módulo comercial opcional, sin generar claves reales:

```bash
CGO_ENABLED=0 go -C src test -tags commercial ./internal/licensing
CGO_ENABLED=0 go -C src test -tags commercial . -run '^TestCommercial'
```

Los resultados de esta revisión y los límites de lo probado están en [VERIFICACION_FINAL.md](VERIFICACION_FINAL.md). Las comprobaciones usan instancias temporales y no necesitan claves de producción.

## Manuales obligatorias antes de desplegar

1. **Primer arranque** en una instalación de prueba nueva (Windows/Fedora),
   permisos de datos y firewall, panel solo loopback.
2. **Migración** con una copia consistente de `gateway.sqlite` y `master.key`;
   confirmar que los tokens/OIDC, sesiones y plantillas permanecen intactos.
3. **Proxy y DNS**: HTTPS LAN y tailnet, servicios, cambios de IP, certificados
   CA local y ACME, limpieza DNS sin tocar registros externos.
4. **API**: revisar usuarios viewer/operator/admin, CSRF, tokens delegados y
   endpoints de Headscale; comprobar también los errores 401/403/404.
5. **PKI**: peer no autorizado, huella diferente, revocación y rechazo de CA.
6. **Backup**: exportación/importación `.hgw`, contraseña equivocada, archivos
   manipulados y restauración completa en equipo de laboratorio.
7. **Windows**: ejecutar, reiniciar desde el panel y repetir como servicio;
   comprobar que no quedan dos procesos con el mismo SQLite abierto.
8. **Logs**: rotación de 3 canales, DEBUG temporal, redacción de secretos y CLI.

La comprobación real del producto exige todas estas pruebas. **No probar
restauraciones destructivas ni políticas ACL de bloqueo sobre la red de
producción** como primer ensayo.
