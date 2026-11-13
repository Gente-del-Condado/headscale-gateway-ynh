package main

import

// Supervisor de vida de la conexión privada: reintentos, identidad,
// listener HTTPS de VPN y cierre ordenado de los recursos tsnet.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"fmt"

	"net"
	"net/http"
	"sync"
	"time"

	"tailscale.com/tsnet"
)

// superviseTailscale mantiene la LAN independiente, controla la vida de tsnet
// y tolera errores de estado transitorios antes de reconstruir la VPN.
func superviseTailscale(cfg Config, requestedName, dataDir, stateDir string, runtime *gatewayRuntime, server *http.Server, provider DNSProvider, vpn *vpnState) {
	const retryInterval = 30 * time.Second
	const healthInterval = 15 * time.Second
	const maxFailures = 3
	first := true
	for {
		if processContext.Err() != nil {
			return
		}
		if !first {
			select {
			case <-time.After(retryInterval):
			case <-processContext.Done():
				return
			}
		}
		first = false
		for vpn.isPaused() {
			select {
			case <-time.After(time.Second):
			case <-processContext.Done():
				return
			}
		}
		if saved, err := loadStoredTailscaleName(dataDir); err == nil && saved != "" {
			requestedName = saved
		}
		tsServer, err := newTailscaleServer(cfg, requestedName, stateDir, func(s *tsnet.Server) {
			vpn.mu.Lock()
			vpn.connecting = s
			vpn.loginURL = ""
			vpn.mu.Unlock()
		}, func(url string) {
			vpn.mu.Lock()
			vpn.loginURL = url
			vpn.mu.Unlock()
		})
		if err != nil {
			vpn.mu.Lock()
			vpn.connecting = nil
			vpn.mu.Unlock()
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-001", "VPN no disponible: %v", err)
			continue
		}

		// La identidad y el listener deben estar preparados antes de publicar DNS.
		dnsName, err := tailscaleDNSName(tsServer)
		if err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-002", "identidad VPN no disponible: %v", err)
			tsServer.Close()
			continue
		}
		actual, err := tailscaleNodeName(dnsName, cfg.BaseDomain)
		if err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-003", "DNSName VPN inválido: %v", err)
			tsServer.Close()
			continue
		}
		listener, err := tsServer.Listen("tcp", ":443")
		if err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-004", "listener VPN: %v", err)
			tsServer.Close()
			continue
		}
		vpn.mu.Lock()
		vpn.server = tsServer
		vpn.connecting = nil
		vpn.mu.Unlock()
		if err := applyVPNIdentity(cfg, tsServer, actual, dataDir, runtime, provider); err != nil {
			vpn.mu.Lock()
			vpn.server = nil
			vpn.mu.Unlock()
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-005", "VPN sin publicar: %v", err)
			listener.Close()
			tsServer.Close()
			continue
		}
		requestedName = actual
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-VPN-SUPERVISOR-006", "VPN activa en :443 para %s", runtime.domain())
		if runtime.showAdminConsole != nil {
			runtime.showAdminConsole()
		}
		stopped := make(chan struct{})
		go func() { serveVPN(server, listener); close(stopped) }()
		ticker := time.NewTicker(healthInterval)
		failures := 0
		candidate := ""
		candidateCount := 0
		running := true
		for running {
			select {
			case <-processContext.Done():
				running = false
			case <-stopped:
				diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-007", "listener VPN finalizado")
				running = false
			case <-ticker.C:
				if vpn.isPaused() {
					failures = 0
					continue
				}
				// Un error aislado no justifica cerrar una VPN que sigue sirviendo.
				currentName, err := inspectVPN(tsServer, cfg.BaseDomain)
				if err != nil {
					failures++
					diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-008", "comprobación VPN fallida (%d/%d): %v", failures, maxFailures, err)
					if failures >= maxFailures {
						running = false
					}
					continue
				}
				failures = 0
				if currentName == runtime.active.Load().name {
					candidate = ""
					candidateCount = 0
					continue
				}
				if candidate != currentName {
					candidate = currentName
					candidateCount = 1
					continue
				}
				candidateCount++
				if candidateCount < 2 {
					continue
				}
				// Cambiar identidad no requiere cerrar un listener que sigue operativo.
				if err := applyVPNIdentity(cfg, tsServer, currentName, dataDir, runtime, provider); err != nil {
					diagnostics.Emit("gateway", diagnostics.Warn, "HGW-VPN-SUPERVISOR-009", "transición de identidad aplazada: %v", err)
					continue
				}
				requestedName = currentName
				candidate = ""
				candidateCount = 0
			}
		}
		ticker.Stop()
		vpn.mu.Lock()
		vpn.server = nil
		vpn.mu.Unlock()
		listener.Close()
		tsServer.Close()
		<-stopped
	}
}

// inspectVPN consulta el estado local una sola vez con timeout y devuelve
// el nombre real. Running no demuestra accesibilidad del control plane.
func inspectVPN(tsServer *tsnet.Server, baseDomain string) (string, error) {
	client, err := tsServer.LocalClient()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, err := client.Status(ctx)
	if err != nil {
		return "", err
	}
	if status == nil || status.BackendState != "Running" || status.Self == nil {
		return "", fmt.Errorf("tsnet no está Running o no tiene identidad")
	}
	return tailscaleNodeName(status.Self.DNSName, baseDomain)
}

func serveVPN(server *http.Server, listener net.Listener) {
	if err := server.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
		diagnostics.Emit("gateway", diagnostics.Error, "HGW-VPN-SUPERVISOR-010",

			// vpnState protege el servidor tsnet durante sincronizaciones DNS manuales.
			"HTTPS VPN: %v", err)
	}
}

type vpnState struct {
	actionMu   sync.Mutex
	paused     bool
	mu         sync.Mutex
	server     *tsnet.Server
	connecting *tsnet.Server
	loginURL   string // URL efímera de registro, protegida por mu; nunca se guarda en la base de datos.
}
