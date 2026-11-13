# Componentes y procedencia

La aplicación y la integración con el motor común pertenecen a Headscale
Gateway. La arquitectura móvil se ha contrastado con TailSocks 5.6.0, leído
desde la copia local facilitada para este trabajo.

El wrapper Gradle 8.13 es una herramienta de construcción; conserva sus
avisos originales en `gradle/wrapper/GRADLE-LICENSE` y `GRADLE-NOTICE`
y verifica la distribución mediante SHA-256. No se incorpora al APK.

| Componente | Uso | Licencia / aviso |
| --- | --- | --- |
| TailSocks | Referencia de ciclo de vida, proceso PIE, red Android y VPN con HEV | BSD de tres cláusulas; `src/third_party/TAILSOCKS-LICENSE` |
| HEV SOCKS5 Tunnel y dependencias incorporadas | Adaptación del TUN a SOCKS5 | Avisos originales conservados en `src/third_party/hev-socks5-tunnel/` |
| Tailscale `tsnet` | Una identidad VPN para el motor y el teléfono | Avisos de la raíz en `src/third_party/go/` |
| `mattn/go-sqlite3` 1.14.52 | SQLite compatible con las llamadas permitidas por Android/Bionic | MIT; aviso de la raíz en `src/third_party/go/github.com/mattn/go-sqlite3@v1.14.52/` |

El adaptador JNI y la aplicación Android se han creado para este proyecto.
La copia de HEV procede de la incluida en TailSocks; no se han incorporado
su actualizador, su administración remota, Root Mode ni sus cuentas de usuario.
No se declara que los autores originales respalden este producto.

La compilación copia los avisos del motor, el inventario Go, las herramientas
y las bibliotecas nativas a `assets/licenses/` dentro de la APK. La pantalla
**Avisos de terceros** presenta la procedencia y los avisos principales.
Los textos legales originales conservan su idioma y contenido; la documentación
y los mensajes propios de la aplicación están en español.
