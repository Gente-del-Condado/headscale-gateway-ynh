package main

import

// Publicación DNS transaccional al confirmar una identidad tsnet.
// Se conserva una generación HTTPS previa si la sincronización falla.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"

	"time"

	"tailscale.com/tsnet"
)

// syncDNSGeneration usa un contexto propio. La limpieza del dominio anterior
// nunca reutiliza un contexto que ya haya consumido el tiempo de sincronización.
func syncDNSGeneration(cfg Config, tsServer *tsnet.Server, snap *gatewaySnapshot, provider DNSProvider) error {
	if provider == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return syncGatewayDNS(ctx, cfg, tsServer, snap.domain, cfg.Services, provider)
}

func cleanupPreviousDNS(cfg Config, provider DNSProvider, previous string) {
	if provider == nil || previous == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := cleanupGatewayDNS(ctx, provider, cfg.DNSZone, previous, map[string]struct{}{}, managedDNSCommentForNode(previous)); err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-DNS-GENERATION-001", "limpieza DNS pendiente para %s: %v", previous, err)
	}
}

// applyVPNIdentity prepara certificados y rutas, confirma DNS y solo después
// publica la generación. Una sincronización fallida impide la transición.
func applyVPNIdentity(cfg Config, tsServer *tsnet.Server, name, dataDir string, runtime *gatewayRuntime, provider DNSProvider) error {
	runtime.transitionMu.Lock()
	defer runtime.transitionMu.Unlock()
	cfg = runtime.currentConfig()
	current := runtime.active.Load()
	next, err := runtime.prepare(name)
	if err != nil {
		return err
	}
	if err := syncDNSGeneration(cfg, tsServer, next, provider); err != nil {
		return err
	}
	runtime.publish(next)
	runtime.identityConfirmed.Store(true)
	if err := saveTailscaleName(dataDir, name); err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-DNS-GENERATION-002", "no se pudo persistir identidad %s: %v", name, err)
	}
	if current != nil && current.domain != next.domain {
		// La limpieza es posterior al commit y no puede invalidar el servicio LAN.
		cleanupPreviousDNS(cfg, provider, current.domain)
	}
	return nil
}
