//go:build windows

package firewall

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func inspect(ctx context.Context, r Report) Report {
	r.Platform = "windows"
	r.Tool = "Windows Defender Firewall"
	rule := fmt.Sprintf("HeadscaleGateway-LAN-%d-TCP", r.Port)
	out, err := exec.CommandContext(ctx, "netsh", "advfirewall", "firewall", "show", "rule", "name="+rule).CombinedOutput()
	if err == nil && strings.Contains(strings.ToLower(string(out)), strings.ToLower(rule)) {
		r.Status = "permitido"
		r.Note = "Existe una regla de Gateway; revisa que esté habilitada y limitada a la red privada"
		return r
	}
	r.Status = "sin_regla"
	r.Note = "No hay una regla administrada por Gateway; pueden existir otras reglas del sistema"
	// Este comando debe ejecutarse en una consola ELEVADA después de revisar
	// puerto y alcance; jamás se ejecuta automáticamente desde el navegador.
	r.Instructions = []string{fmt.Sprintf(`netsh advfirewall firewall add rule name="%s" dir=in action=allow protocol=TCP localport=%d profile=private`, rule, r.Port)}
	return r
}
