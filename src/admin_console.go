package main

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"golang.org/x/term"
)

func adminConsoleInteractive() bool { return term.IsTerminal(int(os.Stderr.Fd())) }

// Un listener exclusivo de proxy no es una dirección visitable por el navegador.
// Esta decisión es independiente de la plataforma que ejecuta el servidor compartido.
func adminConsoleURL(address string, options admin.HTTPOptions) string {
	if len(options.TrustedProxies) != 0 {
		return ""
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + options.BasePath + "/admin/"
}

// Es interfaz de consola, nunca un evento de diagnóstico ni una credencial persistida.
func printAdminConsole(url, code string, pending bool) {
	interactive := adminConsoleInteractive()
	color := interactive && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && enableConsoleANSI()
	fmt.Fprint(os.Stderr, adminConsoleText(url, code, pending, interactive, color))
}

func adminConsoleText(url, code string, pending, interactive, color bool) string {
	var b strings.Builder
	heading, reset := "", ""
	if color {
		heading, reset = "\x1b[1;96m", "\x1b[0m"
	}
	fmt.Fprintf(&b, "\n%s============================================================\n%s\n============================================================%s\n", heading, i18n.T("HEADSCALE GATEWAY · ACCESO AL PANEL"), reset)
	// Las URL simples siguen siendo pulsables en terminales que reconocen enlaces.
	// OSC 8 mejora los terminales compatibles; nunca se incluyen credenciales en la URL.
	link := url
	if color {
		link = "\x1b]8;;" + url + "\x1b\\" + url + "\x1b]8;;\x1b\\"
	}
	if url == "" {
		fmt.Fprintln(&b, i18n.T("Abre el panel desde la interfaz del sistema donde has instalado Gateway."))
		fmt.Fprintln(&b, i18n.T("El puerto HTTP interno está reservado al proxy; no es un enlace de acceso directo."))
		fmt.Fprintln(&b)
	} else {
		fmt.Fprintf(&b, "%s\n  %s\n\n", i18n.T("Abre este enlace en el navegador de este equipo:"), link)
	}
	if code != "" && interactive {
		fmt.Fprintf(&b, "%s\n\n  %s%s%s\n\n%s\n%s\n", i18n.T("CÓDIGO TEMPORAL DE RECUPERACIÓN"), heading, code, reset,
			i18n.T("Un solo uso. Caduca en 15 minutos desde este momento."),
			i18n.T("Introduce el código en el acceso de recuperación del panel; después cambia tu contraseña en Acceso / Identidad."))
	} else if pending {
		fmt.Fprintln(&b, i18n.T("Código pendiente: inicia Gateway en una terminal interactiva, sin redirigir la salida de errores. No se escribe el código en registros."))
	} else {
		fmt.Fprintln(&b, i18n.T("Accede con tu cuenta. Para recuperar el acceso: detén Gateway, ejecuta --reset-recovery con la misma opción --db y vuelve a iniciarlo."))
	}
	fmt.Fprintln(&b, i18n.T("La recuperación no cambia por sí sola la contraseña de tu cuenta."))
	fmt.Fprintf(&b, "%s============================================================%s\n\n", heading, reset)
	return b.String()
}
