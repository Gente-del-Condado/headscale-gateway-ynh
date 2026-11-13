//go:build linux && !android

package firewall

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func inspect(ctx context.Context, r Report) Report {
	r.Platform = "linux"
	port := fmt.Sprintf("%d/tcp", r.Port)
	if _, err := exec.LookPath("firewall-cmd"); err == nil {
		// Que una consulta falle no significa que no exista otra regla NAT,
		// rich-rule o una política externa que permita el acceso.
		if exec.CommandContext(ctx, "firewall-cmd", "--quiet", "--query-port", port).Run() == nil {
			r.Status = "permitido"
			r.Tool = "firewalld"
			r.Note = "Puerto permitido en la zona predeterminada"
			return r
		}
		r.Tool = "firewalld"
		r.Status = "sin_regla"
		r.Note = "No hay regla directa en la zona predeterminada"
		r.Instructions = []string{fmt.Sprintf("sudo firewall-cmd --add-port=%s --permanent", port), "sudo firewall-cmd --reload"}
		return r
	}
	if _, err := exec.LookPath("ufw"); err == nil {
		r.Tool = "ufw"
		r.Note = "Comprobar estado y reglas efectivas de UFW"
		r.Instructions = []string{"sudo ufw status verbose", fmt.Sprintf("sudo ufw allow %s", port)}
		if data, err := exec.CommandContext(ctx, "ufw", "status").Output(); err == nil && strings.Contains(string(data), port) {
			r.Status = "permitido"
		}
		return r
	}
	r.Note = "No se ha encontrado firewalld ni ufw; comprueba nftables/iptables o el cortafuegos del router"
	r.Instructions = []string{"sudo nft list ruleset"}
	return r
}
