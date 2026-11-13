//go:build darwin

package firewall

import "context"

func inspect(_ context.Context, r Report) Report {
	r.Platform = "darwin"
	r.Tool = "Application Firewall"
	r.Note = "macOS regula aplicaciones, no solo puertos. Comprueba Ajustes del Sistema → Red → Firewall y autoriza el binario firmado de Gateway"
	return r
}
