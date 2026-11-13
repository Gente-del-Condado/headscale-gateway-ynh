package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"github.com/gentedelcondado/headscale-gateway/internal/policytemplates"
	"github.com/gentedelcondado/headscale-gateway/internal/templatelib"
)

const policyCatalogKey = "acl.policy_templates"
const policyRepoURLKey = "acl.templates_repository_url"
const policyRepoPrivateKey = "acl.templates_repository_private"

func (a *adminController) PolicyTemplates(_ context.Context) ([]policytemplates.Template, error) {
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
	return unpackPolicyTemplates(values[policyCatalogKey])
}
func unpackPolicyTemplates(raw string) ([]policytemplates.Template, error) {
	out := policytemplates.Builtins()
	if raw == "" {
		return out, nil
	}
	var stored []policytemplates.Template
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, err
	}
	for _, v := range stored {
		if err := policytemplates.Validate(v); err != nil {
			return nil, err
		}
		if v.Source == "builtin" {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
func (a *adminController) StorePolicyTemplates(_ context.Context, bundle policytemplates.Bundle, overwrite bool) (admin.TemplateImportResult, error) {
	result := admin.TemplateImportResult{}
	raw, err := json.Marshal(bundle)
	if err != nil {
		return result, err
	}
	if _, err = policytemplates.Decode(raw); err != nil {
		return result, err
	}
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return result, err
	}
	defer db.Close()
	settings, err := db.GetSettings()
	if err != nil {
		return result, err
	}
	var stored []policytemplates.Template
	if settings[policyCatalogKey] != "" {
		if err = json.Unmarshal([]byte(settings[policyCatalogKey]), &stored); err != nil {
			return result, err
		}
	}
	ids := map[string]int{}
	for idx, item := range stored {
		ids[item.ID] = idx
	}
	for _, item := range bundle.Templates {
		reserved := false
		for _, builtin := range policytemplates.Builtins() {
			if item.ID == builtin.ID {
				reserved = true
				break
			}
		}
		if reserved {
			result.Skipped++
			continue
		}
		if idx, exists := ids[item.ID]; exists {
			if !overwrite {
				result.Skipped++
				continue
			}
			item.Source = "imported"
			stored[idx] = item
			result.Updated++
		} else {
			item.Source = "imported"
			ids[item.ID] = len(stored)
			stored = append(stored, item)
			result.Added++
		}
	}
	if len(stored) > policytemplates.MaxStoredTemplates {
		return admin.TemplateImportResult{}, errors.New("demasiadas plantillas")
	}
	serialized, err := json.Marshal(stored)
	if err != nil {
		return admin.TemplateImportResult{}, err
	}
	if len(serialized) > policytemplates.MaxStoredBytes {
		return admin.TemplateImportResult{}, errors.New("las plantillas no caben en el catálogo SQLite (14 KiB)")
	}
	settings[policyCatalogKey] = string(serialized)
	if err = db.ReplaceSettings(settings); err != nil {
		return admin.TemplateImportResult{}, err
	}
	diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-ACL-TPL-001", "plantillas ACL importadas: nuevas=%d actualizadas=%d omitidas=%d", result.Added, result.Updated, result.Skipped)
	return result, nil
}
func (a *adminController) DeletePolicyTemplate(_ context.Context, id string) error {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	settings, err := db.GetSettings()
	if err != nil {
		return err
	}
	var stored []policytemplates.Template
	if settings[policyCatalogKey] == "" {
		return errors.New("plantilla no encontrada o integrada en el programa")
	}
	if err = json.Unmarshal([]byte(settings[policyCatalogKey]), &stored); err != nil {
		return err
	}
	index := -1
	for i, v := range stored {
		if v.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("plantilla no encontrada o integrada en el programa")
	}
	stored = append(stored[:index], stored[index+1:]...)
	raw, _ := json.Marshal(stored)
	settings[policyCatalogKey] = string(raw)
	return db.ReplaceSettings(settings)
}
func (a *adminController) PolicyRepoSettings(_ context.Context) (admin.TemplateRepositorySettings, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return admin.TemplateRepositorySettings{}, err
	}
	defer db.Close()
	settings, err := db.GetSettings()
	if err != nil {
		return admin.TemplateRepositorySettings{}, err
	}
	return admin.TemplateRepositorySettings{URL: settings[policyRepoURLKey], AllowPrivate: settings[policyRepoPrivateKey] == "true"}, nil
}
func (a *adminController) SavePolicyRepoSettings(_ context.Context, next admin.TemplateRepositorySettings) error {
	next.URL = strings.TrimSpace(next.URL)
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
	settings, err := db.GetSettings()
	if err != nil {
		return err
	}
	settings[policyRepoURLKey] = next.URL
	settings[policyRepoPrivateKey] = fmt.Sprintf("%t", next.AllowPrivate)
	return db.ReplaceSettings(settings)
}
func (a *adminController) FetchPolicyRepo(ctx context.Context) (policytemplates.Bundle, error) {
	settings, err := a.PolicyRepoSettings(ctx)
	if err != nil {
		return policytemplates.Bundle{}, err
	}
	if settings.URL == "" {
		return policytemplates.Bundle{}, errors.New("primero configura la URL HTTPS del repositorio")
	}
	raw, err := templatelib.Fetch(ctx, settings.URL, settings.AllowPrivate)
	if err != nil {
		return policytemplates.Bundle{}, err
	}
	return policytemplates.Decode(raw)
}
func (a *adminController) SyncPolicyRepo(ctx context.Context, overwrite bool) (admin.TemplateImportResult, error) {
	b, err := a.FetchPolicyRepo(ctx)
	if err != nil {
		return admin.TemplateImportResult{}, err
	}
	return a.StorePolicyTemplates(ctx, b, overwrite)
}
