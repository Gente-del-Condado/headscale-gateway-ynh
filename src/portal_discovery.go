package main

import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net"
	"time"
)

func (a *adminController) portalDiscoveryLoop(ctx context.Context, db *state.Store, listen string) {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var stop context.CancelFunc
	var done chan error
	var retry time.Time
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-done:
			stop = nil
			done = nil
			retry = time.Now().Add(time.Minute)
			if err != nil && ctx.Err() == nil {
				diagnostics.Emit("admin", diagnostics.Warn, "HGW-DISCOVERY-LAN", "Anuncio LAN no disponible: %v", err)
			}
		case <-ticker.C:
			cfg := a.runtime.currentConfig()
			var publicCA bool
			_, _ = db.GetJSON("pki.public_ca_enabled_v1", &publicCA)
			enabled := cfg.Portal.Discovery && cfg.Portal.Publish || publicCA && (cfg.AdminDomainMode == "all" || a.runtime.caHTTPPort.Load() > 0)
			if !enabled && stop != nil {
				stop()
				stop = nil
				done = nil
			}
			if enabled && stop == nil && time.Now().After(retry) {
				child, cancel := context.WithCancel(ctx)
				stop = cancel
				done = make(chan error, 1)
				result := done
				go func() {
					result <- pki.ServeDiscovery(child, func() string {
						domain := a.runtime.domain()
						if domain == "" {
							return ""
						}
						if port != "443" {
							domain = net.JoinHostPort(domain, port)
						}
						return "https://" + domain
					}, func() int {
						var public bool
						_, _ = db.GetJSON("pki.public_ca_enabled_v1", &public)
						if !public {
							return 0
						}
						return int(a.runtime.caHTTPPort.Load())
					}, cfg.DiscoveryPort)
				}()
			}
		}
	}
}
