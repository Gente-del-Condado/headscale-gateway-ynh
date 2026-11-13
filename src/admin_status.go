package main

// Proyección del estado de configuración y del estado de salud en tiempo de ejecución para REST.
import (
	"context"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/cli"
	"time"
)

func (a *adminController) Status(_ context.Context) (any, error) {
	cfg := a.runtime.currentConfig()
	snap := a.runtime.active.Load()
	a.vpn.mu.Lock()
	online := a.vpn.server != nil
	a.vpn.mu.Unlock()
	result := map[string]any{
		"version": cli.Version,
		"node":    "", "domain": "", "services_active": len(cfg.Services),
		"vpn_connected": online, "certificate_mode": cfg.CertificateMode,
		"dns_provider": cfg.DNSProvider, "headscale_url": cfg.HeadscaleURL,
		"uptime_seconds": int(time.Since(a.started).Seconds()), "boot_id": a.started.UTC().Format(time.RFC3339Nano), "data_dir": a.runtime.dataDir,
		"lan_https_active":  a.runtime.lanListening.Load(),
		"ca_http_port":      a.runtime.caHTTPPort.Load(),
		"setup_required":    snap == nil,
		"admin_domain_mode": cfg.AdminDomainMode,
	}
	if a.runtime.license != nil {
		result["subscription"] = a.runtime.license.Status()
	}
	if snap != nil {
		if cfg.AdminEnabled && cfg.AdminDomainMode != "off" {
			url := "https://" + cfg.AdminSubdomain + "." + snap.domain
			result["admin_domain_url"] = url + "/admin/"
			result["admin_oidc_callback"] = url + "/admin/auth/oidc/callback"
		}
		result["node"] = snap.name
		result["domain"] = snap.domain
		result["vpn_identity_confirmed"] = a.runtime.identityConfirmed.Load()
		if snap.certificates != nil {
			switch {
			case cfg.CertificateMode == "force-acme" && snap.certificates.localTLSCertificate != nil:
				result["certificate_source"] = "pending-acme"
			case cfg.CertificateMode == "force-local":
				result["certificate_source"] = "local-ca"
			case snap.certificates.localTLSCertificate != nil:
				result["certificate_source"] = "local-fallback"
			default:
				result["certificate_source"] = "acme"
			}
		}
		if cfg.CertificateMode == "auto" {
			if until := readACMEBackoff(cfg, snap.domain); until.After(time.Now()) {
				result["acme_retry_after_utc"] = until.UTC().Format(time.RFC3339)
			}
		}
	}
	if snap == nil {
		if reason := setupReason(cfg); reason != nil {
			result["setup_message"] = reason.Error()
		}
	}
	return result, nil
}

// Services lee el catálogo; los servicios deshabilitados permanecen visibles para su edición.

// RefreshCertificate cuenta, de forma intencionada, con privilegios especiales al tratarse de una operación de configuración.
// El límite de frecuencia (rate limit) nunca se omite, ni siquiera cuando se solicita explícitamente desde la interfaz de usuario.
func (a *adminController) RefreshCertificate(_ context.Context) (any, error) {
	if !a.runtime.identityConfirmed.Load() {
		return nil, fmt.Errorf("la identidad VPN todavía no está confirmada")
	}
	if err := a.runtime.refreshACME(); err != nil {
		return nil, err
	}
	return a.Status(context.Background())
}
