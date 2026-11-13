package main

import (
	"fmt"
	"strings"
)

// En modo inicial se admiten campos globales obligatorios incompletos,
// pero las demás reglas de seguridad/validación se siguen aplicando.
func validateStagedConfig(cfg Config) error {
	candidate := cfg
	if candidate.BaseDomain == "" {
		candidate.BaseDomain = "configuracion.example.invalid"
	}
	if candidate.HeadscaleURL == "" {
		candidate.HeadscaleURL = "https://headscale.example.invalid"
	}
	if candidate.ACMEEmail == "" && candidate.CertificateMode != "force-local" {
		candidate.ACMEEmail = "setup@example.invalid"
	}
	return validateConfig(candidate)
}

func setupReason(cfg Config) error {
	if cfg.BaseDomain == "" || cfg.HeadscaleURL == "" {
		missing := []string{}
		if cfg.BaseDomain == "" {
			missing = append(missing, "domain.base_domain")
		}
		if cfg.HeadscaleURL == "" {
			missing = append(missing, "headscale.url")
		}
		return fmt.Errorf("configuración pendiente: %s", strings.Join(missing, ", "))
	}
	return validateConfig(cfg)
}
