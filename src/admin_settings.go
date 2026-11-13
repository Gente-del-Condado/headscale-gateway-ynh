package main

// Actualizaciones validadas y filtrado de secretos para la configuración global de la puerta de enlace.
import (
	"context"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
)

func (a *adminController) Settings(_ context.Context) (any, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	values, err := db.GetSettings()
	if err != nil {
		return nil, err
	}
	for k, v := range initialSettings() {
		if _, ok := values[k]; !ok {
			values[k] = v
		}
	}
	for _, k := range []string{"headscale.api_key", "dns.technitium.token", "dns.cloudflare.token", "acme.cloudflare.token"} {
		values[k] = ""
	}
	return values, nil
}

var allowedSettings = map[string]bool{
	"pki.discovery_port": true,
	"pki.http_enabled":   true, "pki.http_listen_addr": true, "pki.allow_http_download": true,
	"server.listen_addr": true, "domain.base_domain": true, "headscale.url": true,
	"headscale.api_key": true, "headscale.panel_enabled": true, "dns.provider": true,
	"dns.zone": true, "dns.technitium.url": true, "dns.technitium.token": true,
	"dns.cloudflare.zone_id": true, "dns.cloudflare.token": true,
	"acme.cloudflare.zone_id": true, "acme.cloudflare.token": true,
	"acme.dns_provider": true, "acme.email": true, "certificates.mode": true,
	"certificates.renew_before": true, "certificates.local_ca_validity": true,
	"certificates.local_certificate_validity": true, "services.default_template": true, "proxy.default_egress": true,
	"admin.enabled": true, "admin.listen_addr": true,
	"admin.domain_mode": true, "admin.subdomain": true,
}
var secretSettings = map[string]bool{"headscale.api_key": true, "dns.technitium.token": true, "dns.cloudflare.token": true, "acme.cloudflare.token": true}

func (a *adminController) UpdateSettings(_ context.Context, changes map[string]string) error {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	current, err := db.GetSettings()
	if err != nil {
		return err
	}
	next := make(map[string]string, len(current)+len(changes))
	for k, v := range current {
		next[k] = v
	}
	if err = a.validateSettingChoicesLocked(changes); err != nil {
		return err
	}
	for k, v := range changes {
		if !allowedSettings[k] {
			return fmt.Errorf("opción no editable: %q", k)
		}
		if len(v) > 16384 {
			return fmt.Errorf("valor demasiado largo: %s", k)
		}
		if secretSettings[k] && v == "" {
			continue
		}
		next[k] = v
	}
	candidate, err := configFromSettings(a.configPath, next, db)
	if err == nil {
		if a.runtime.active.Load() == nil {
			err = validateStagedConfig(candidate)
		} else {
			err = validateConfig(candidate)
		}
	}
	if err != nil {
		return fmt.Errorf("configuración rechazada: %w", err)
	}
	if err = db.ReplaceSettings(next); err != nil {
		return err
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-ADMIN-SETTINGS-001", "configuración SQLite validada y guardada: reinicio requerido")
	return nil
}
