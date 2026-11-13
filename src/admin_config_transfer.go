package main

import (
	"context"
	"errors"
	"slices"

	"github.com/gentedelcondado/headscale-gateway/internal/configtransfer"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
)

// ExportConfiguration genera un perfil portátil; nunca exporta tokens de
// sesión, master.key, claves privadas de la CA ni la identidad tsnet.
func (a *adminController) ExportConfiguration(_ context.Context, secrets bool, password string) ([]byte, error) {
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
	allowed := make(map[string]string)
	for k, v := range values {
		if !allowedSettings[k] || (!secrets && secretSettings[k]) {
			continue
		}
		allowed[k] = v
	}
	return configtransfer.Export(allowed, secrets, password)
}

// ImportConfiguration conserva siempre los listeners y mecanismos locales de
// recuperación. Valida toda la propuesta antes de modificar SQLite.
func (a *adminController) ImportConfiguration(ctx context.Context, payload []byte, password string) (int, error) {
	changes, err := portableChanges(payload, password)
	if err != nil {
		return 0, err
	}
	if err = a.UpdateSettings(ctx, changes); err != nil {
		return 0, err
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-CONFIG-IMPORT", "opciones importadas=%d; reinicio requerido", len(changes))
	return len(changes), nil
}

// PreviewConfiguration enumera solo las claves aplicables del perfil.
func (a *adminController) PreviewConfiguration(payload []byte, password string) ([]string, error) {
	changes, err := portableChanges(payload, password)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(changes))
	for key := range changes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys, nil
}

func portableChanges(payload []byte, password string) (map[string]string, error) {
	profile, err := configtransfer.Import(payload, password)
	if err != nil {
		return nil, err
	}
	changes := make(map[string]string)
	for key, value := range profile.Settings {
		if !allowedSettings[key] {
			return nil, errors.New("la configuración contiene una clave desconocida")
		}
		if len(value) > 16384 {
			return nil, errors.New("valor de configuración demasiado grande")
		}
		switch key {
		case "pki.http_enabled", "pki.http_listen_addr", "pki.discovery_port", "pki.allow_http_download", "server.listen_addr", "admin.listen_addr", "admin.enabled", "admin.domain_mode", "admin.subdomain":
			// No cortar el acceso administrativo de destino al migrar ajustes.
			continue
		}
		if secretSettings[key] && value == "" {
			continue
		}
		changes[key] = value
	}
	if len(changes) == 0 {
		return nil, errors.New("el perfil no contiene opciones importables")
	}
	return changes, nil
}
