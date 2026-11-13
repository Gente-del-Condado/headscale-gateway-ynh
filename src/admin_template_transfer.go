package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"github.com/gentedelcondado/headscale-gateway/internal/templatelib"
)

type templateBundle = templatelib.Bundle[state.TemplateRecord]
type templateImportResult = admin.TemplateImportResult
type templateRepositorySettings = admin.TemplateRepositorySettings

func validateImportedTemplate(item state.TemplateRecord) error {
	if !validAdminServiceName(item.Name) || len(item.Proxy) == 0 || len(item.Proxy) > 32768 || len(item.Category) > 80 || len(item.Description) > 1024 || len(item.BackendHint) > 256 {
		return fmt.Errorf("plantilla %q: nombre o tamaño inválido", item.Name)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(item.Proxy, &raw); err != nil || raw == nil {
		return fmt.Errorf("plantilla %q: proxy debe ser un objeto JSON", item.Name)
	}
	var options storedProxyOptions
	if err := json.Unmarshal(item.Proxy, &options); err != nil {
		return fmt.Errorf("plantilla %q: %w", item.Name, err)
	}
	if _, err := parseProxyOptions(options); err != nil {
		return fmt.Errorf("plantilla %q: %w", item.Name, err)
	}
	return nil
}

func (a *adminController) ExportTemplates(ctx context.Context) (templateBundle, error) {
	rows, err := a.TemplateDetails(ctx)
	if err != nil {
		return templateBundle{}, err
	}
	return templateBundle{Format: templatelib.Format, Version: 1, Templates: rows}, nil
}

// ImportTemplates valida todo el lote antes de escribir y restaura el catálogo
// original si fallan SQLite o la recarga. Nunca elimina plantillas existentes,
// incluso con overwrite=true.
func (a *adminController) ImportTemplates(_ context.Context, bundle templateBundle, overwrite bool) (templateImportResult, error) {
	result := templateImportResult{}
	if bundle.Format != templatelib.Format || bundle.Version != 1 || len(bundle.Templates) == 0 || len(bundle.Templates) > templatelib.MaxTemplates {
		return result, errors.New("formato o número de plantillas no admitido")
	}
	unique := make(map[string]bool)
	for _, item := range bundle.Templates {
		if unique[item.Name] {
			return result, fmt.Errorf("plantilla duplicada: %s", item.Name)
		}
		unique[item.Name] = true
		if err := validateImportedTemplate(item); err != nil {
			return result, err
		}
	}
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return result, err
	}
	defer db.Close()
	before, err := db.ListTemplates()
	if err != nil {
		return result, err
	}
	originals := make(map[string]state.TemplateRecord, len(before))
	for _, item := range before {
		originals[item.Name] = item
	}
	changed := make([]string, 0, len(bundle.Templates))
	restore := func() {
		for _, name := range changed {
			if prev, found := originals[name]; found {
				_ = db.SaveTemplate(prev)
			} else {
				_ = db.DeleteTemplate(name)
			}
		}
		_ = a.reloadOrValidate()
	}
	for _, item := range bundle.Templates {
		_, exists := originals[item.Name]
		if exists && !overwrite {
			result.Skipped++
			continue
		}
		if err := db.SaveTemplate(item); err != nil {
			restore()
			return templateImportResult{}, fmt.Errorf("guardando plantilla %s: %w", item.Name, err)
		}
		changed = append(changed, item.Name)
		if exists {
			result.Updated++
		} else {
			result.Added++
		}
	}
	if len(changed) > 0 {
		if err := a.reloadOrValidate(); err != nil {
			restore()
			return templateImportResult{}, fmt.Errorf("importación revertida: %w", err)
		}
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-TPL-IMPORT-001", "plantillas importadas: nuevas=%d actualizadas=%d omitidas=%d", result.Added, result.Updated, result.Skipped)
	return result, nil
}

func (a *adminController) RepositorySettings(_ context.Context) (templateRepositorySettings, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return templateRepositorySettings{}, err
	}
	defer db.Close()
	settings, err := db.GetSettings()
	if err != nil {
		return templateRepositorySettings{}, err
	}
	return templateRepositorySettings{URL: settings["templates.repository_url"], AllowPrivate: settings["templates.repository_allow_private"] == "true"}, nil
}
func (a *adminController) SaveRepositorySettings(_ context.Context, next templateRepositorySettings) error {
	if next.URL != "" {
		if err := templatelib.ValidateURL(next.URL); err != nil {
			return err
		}
	}
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	values, err := db.GetSettings()
	if err != nil {
		return err
	}
	values["templates.repository_url"] = strings.TrimSpace(next.URL)
	values["templates.repository_allow_private"] = fmt.Sprintf("%t", next.AllowPrivate)
	if err := db.ReplaceSettings(values); err != nil {
		return err
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-TPL-REPO-001", "configuración de repositorio modificada; acceso_privado=%t", next.AllowPrivate)
	return nil
}
func (a *adminController) DownloadRepository(ctx context.Context) (templateBundle, error) {
	repo, err := a.RepositorySettings(ctx)
	if err != nil {
		return templateBundle{}, err
	}
	if repo.URL == "" {
		return templateBundle{}, errors.New("configure primero la URL del repositorio")
	}
	raw, err := templatelib.Fetch(ctx, repo.URL, repo.AllowPrivate)
	if err != nil {
		return templateBundle{}, err
	}
	return templatelib.Decode[state.TemplateRecord](raw)
}
func (a *adminController) SyncRepository(ctx context.Context, overwrite bool) (templateImportResult, error) {
	bundle, err := a.DownloadRepository(ctx)
	if err != nil {
		diagnostics.Emit("admin", diagnostics.Error, "HGW-TPL-SYNC-ERR", "fallo descarga de repositorio: %v", err)
		return templateImportResult{}, err
	}
	result, err := a.ImportTemplates(ctx, bundle, overwrite)
	if err != nil {
		diagnostics.Emit("admin", diagnostics.Error, "HGW-TPL-SYNC-ERR", "fallo importación: %v", err)
	} else {
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-TPL-SYNC-001", "%s", i18n.T("sincronización finalizada"))
	}
	return result, err
}
