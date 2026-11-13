package main

import

// Recarga transaccional sin reinicio: prepara TLS y proxy, sincroniza DNS
// y publica el nuevo snapshot solo tras comprobar sus dependencias.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"

	"reflect"
)

// reloadServices aplica únicamente cambios de servicios/plantillas.
// Los parámetros globales necesitan reiniciar el proceso.
func (r *gatewayRuntime) reloadServices(configPath string, vpn *vpnState, provider DNSProvider) error {
	r.transitionMu.Lock()
	defer r.transitionMu.Unlock()
	return r.reloadServicesLocked(configPath, vpn, provider)
}

// reloadServicesLocked exige transitionMu adquirido; permite que los cambios
// de archivos y su publicación formen una sola operación administrativa.
func (r *gatewayRuntime) reloadServicesLocked(configPath string, vpn *vpnState, provider DNSProvider) error {
	next, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	if err := validateConfig(next); err != nil {
		return err
	}
	previous := r.currentConfig()
	if !sameGlobalConfig(previous, next) {
		return fmt.Errorf("la recarga solo permite modificar services/ y templates/; los ajustes globales requieren reinicio")
	}
	current := r.active.Load()
	candidateConfig := next
	if !r.identityConfirmed.Load() {
		// La recarga de servicios durante el login de tsnet tampoco debe
		// desencadenar pedidos ACME para un nombre provisional.
		candidateConfig.CertificateMode = "force-local"
	}
	candidate, err := r.prepareConfig(current.name, candidateConfig)
	if err == nil && !r.identityConfirmed.Load() && next.CertificateMode == "force-acme" {
		// Se conserva el TLS bloqueado del primer inicio (force-acme).
		// Nunca publicar el certificado local como sustituto.
		candidate.tlsConfig = current.tlsConfig
	}
	if err != nil {
		return err
	}
	vpn.mu.Lock()
	defer vpn.mu.Unlock()
	if provider != nil && vpn.server != nil {
		if err := syncDNSGeneration(next, vpn.server, candidate, provider); err != nil {
			// Mejor esfuerzo: restaurar el conjunto de registros anterior.
			if rollbackErr := syncDNSGeneration(previous, vpn.server, current, provider); rollbackErr != nil {
				diagnostics.Emit("gateway", diagnostics.Warn, "HGW-RUNTIME-RELOAD-001", "rollback DNS fallido: %v", rollbackErr)
			}
			return fmt.Errorf("sincronización DNS fallida, configuración anterior conservada: %w", err)
		}
	}
	r.cfg.Store(&next)
	r.publish(candidate)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-RUNTIME-RELOAD-002", "Recarga manual completada: %d servicios activos", len(next.Services))
	return nil
}

func sameGlobalConfig(a, b Config) bool {
	a.Services = nil
	a.ServiceProxy = nil
	b.Services = nil
	b.ServiceProxy = nil
	return reflect.DeepEqual(a, b)
}
