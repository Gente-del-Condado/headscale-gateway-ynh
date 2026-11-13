package main

import

// Arranque del servicio tras validar SQLite y resolver los ajustes.
// Esta fase conserva separados el modo de configuración inicial,
// el listener HTTPS LAN y la conexión tsnet (que se supervisa aparte).
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"github.com/gentedelcondado/headscale-gateway/internal/licensing"
	"net/http"
	"os"
	"path/filepath"
)

func startGateway(configPath string) {
	cfg, err := loadConfig(configPath)
	if err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-001",

			// Diagnóstico de red antes del primer listener; no modifica reglas.
			"No se puede continuar: %v", err)
	}

	reviewFirewallStartup(cfg.LocalListenAddr)
	if reason := setupReason(cfg); reason != nil {
		if !cfg.AdminEnabled {
			diagnostics.Fatal("HGW-STARTUP-GATEWAY-002", "%v; active admin.enabled para configurar el Gateway", reason)
		}
		dataDir, err := getDataDir(cfg)
		if err != nil {
			diagnostics.Fatal("HGW-STARTUP-GATEWAY-003", "No se puede continuar: %v", err)
		}
		runtime := &gatewayRuntime{dataDir: dataDir, license: licensing.New(dataDir)}
		runtime.cfg.Store(&cfg)
		vpn, vpnErr := loadVPNState(configPath)
		if vpnErr != nil {
			diagnostics.Fatal("HGW-STARTUP-GATEWAY-004", "No se puede continuar: %v", vpnErr)
		}
		if err := startAdministration(cfg, configPath, runtime, vpn, nil); err != nil {
			diagnostics.Fatal("HGW-STARTUP-GATEWAY-005", "No se puede continuar: %v", err)
		}
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-STARTUP-GATEWAY-006", "%v. Solo API/panel activos; VPN, TLS, DNS y proxy no se iniciaron", reason)
		// El panel permanece operativo hasta que se guarden los ajustes y se
		// pulse Reiniciar. Las señales del sistema terminan el proceso.
		runtime.showAdminConsole()
		waitForShutdown()
		return
	}
	hostname, err := os.Hostname()
	if err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-007", "No se puede continuar: %v", err)
	}
	hostname = normalizeDNSName(hostname)
	options, err := loadStartupOptions()
	if err != nil {
		diagnostics.Fatal("HGW-DEPLOYMENT", "%v", err)
	}
	hostname, err = stateHostname(configPath, options, hostname)
	if err != nil {
		diagnostics.Fatal("HGW-DEPLOYMENT", "%v", err)
	}
	dataDir, err := getDataDir(cfg)
	if err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-008", "No se puede continuar: %v", err)
	}
	storedName, err := loadStoredTailscaleName(dataDir)
	if err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-009", "No se puede continuar: %v", err)
	}
	requestedName := hostname
	if storedName != "" {
		requestedName = storedName
	}
	stateDir := filepath.Join(dataDir, "tailscale-state-"+hostname)
	if err := mkdirPrivate(stateDir); err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-010",

			// La LAN se prepara y se abre ANTES de cualquier operación de red con tsnet.
			// El nombre almacenado es preferible; sin él se usa el hostname provisional.
			"No se puede continuar: %v", err)
	}

	vpn, vpnErr := loadVPNState(configPath)
	if vpnErr != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-011", "No se puede continuar: %v", vpnErr)
	}
	runtime, err := newGatewayRuntime(cfg, dataDir, requestedName)
	if err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-012", "No se puede continuar: %v", err)
	}
	runtime.vpn = vpn
	provider, err := newDNSProvider(cfg)
	if err != nil {
		diagnostics.Fatal("HGW-STARTUP-GATEWAY-013",

			// Servidores independientes para distinguir una conexión aceptada por tsnet
			// de una conexión LAN. Las cabeceras X-Forwarded-* NO conceden privilegios.
			"No se puede continuar: %v", err)
	}

	// Mantiene disponible la administración aunque el puerto HTTPS LAN esté ocupado.
	if cfg.AdminEnabled {
		if err := startAdministration(cfg, configPath, runtime, vpn, provider); err != nil {
			diagnostics.Fatal("HGW-STARTUP-GATEWAY-017", "No se puede continuar: %v", err)
		}
	}
	server := newGatewayHTTPServer(runtime, false)
	vpnServer := newGatewayHTTPServer(runtime, true)
	lan, err := listenLocal(cfg.LocalListenAddr)
	if err != nil {
		if !cfg.AdminEnabled {
			diagnostics.Fatal("HGW-STARTUP-GATEWAY-014", "No se puede continuar: %v", err)
		}
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-LAN-PORT-BUSY", "HTTPS LAN no disponible; cambia su puerto desde administración y reinicia: %v", err)
	} else {
		defer lan.Close()
		runtime.lanListening.Store(true)
		go func() {
			defer runtime.lanListening.Store(false)
			if err := server.ServeTLS(lan, "", ""); err != nil && err != http.ErrServerClosed {
				diagnostics.Emit("gateway", diagnostics.Error, "HGW-STARTUP-GATEWAY-015", "HTTPS LAN: %v", err)
			}
		}()
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-STARTUP-GATEWAY-016", "HTTPS LAN activo en %s; dominio inicial %s", cfg.LocalListenAddr, runtime.domain())
	}
	go watchReloadRequests(configPath, dataDir, runtime, vpn, provider)
	go refreshCertificatesPeriodically(runtime)
	shutdownTasks.Add(1)
	go func() {
		defer shutdownTasks.Done()
		superviseTailscale(cfg, requestedName, dataDir, stateDir, runtime, vpnServer, provider, vpn)
	}()
	waitForShutdown()
	shutdownHTTPServer(server)
	shutdownHTTPServer(vpnServer)
}
