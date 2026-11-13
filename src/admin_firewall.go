package main

import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/platform/firewall"
)

// FirewallReport examina la regla de entrada LAN activa; el puerto 443 de
// tsnet viaja dentro de la VPN y no requiere abrir la entrada física.
func (a *adminController) FirewallReport(ctx context.Context) (firewall.Report, error) {
	cfg := a.runtime.currentConfig()
	return firewall.Inspect(ctx, cfg.LocalListenAddr)
}
