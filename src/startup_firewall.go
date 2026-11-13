package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"

	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/platform/firewall"
)

// En el primer inicio y en cada arranque se informa si el HTTPS LAN necesita
// una autorización de entrada. No toca el cortafuegos sin aprobación explícita.
// El puerto de recuperación HTTP (9091) debe permanecer siempre en loopback.
func reviewFirewallStartup(listen string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	report, err := firewall.Inspect(ctx, listen)
	if err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-FIREWALL", "diagnóstico no disponible: %v", err)
		return
	}
	if report.Remote && report.Status != "permitido" {
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-FIREWALL", "HTTPS LAN %s: %s. Revisa Configuración → Cortafuegos. No se ha modificado ninguna regla.", listen, report.Note)
	}
}
