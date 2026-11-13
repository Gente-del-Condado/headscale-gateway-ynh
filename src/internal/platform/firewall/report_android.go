//go:build android

package firewall

import "context"

func inspect(_ context.Context, r Report) Report {
	r.Platform = "android"
	r.Note = "Android: comprueba los permisos de red, la VPN activa y el cortafuegos de la red Wi-Fi. No se cambian reglas del sistema desde la aplicación."
	r.Instructions = []string{"Revisa los ajustes de la aplicación y las restricciones de batería y datos de Android"}
	return r
}
