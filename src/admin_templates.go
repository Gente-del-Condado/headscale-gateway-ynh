package main

// Operaciones CRUD para plantillas de proxy reutilizables en SQLite.
import (
	"context"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

func (a *adminController) TemplateDetails(_ context.Context) ([]state.TemplateRecord, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.ListTemplates()
}
func (a *adminController) SaveTemplate(_ context.Context, item state.TemplateRecord) error {
	if err := validateImportedTemplate(item); err != nil {
		return err
	}
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	previous, err := db.ListTemplates()
	if err != nil {
		return err
	}
	old := state.TemplateRecord{}
	existed := false
	for _, row := range previous {
		if row.Name == item.Name {
			old = row
			existed = true
			break
		}
	}
	if err = db.SaveTemplate(item); err != nil {
		return err
	}
	if err = a.reloadOrValidate(); err != nil {
		if existed {
			_ = db.SaveTemplate(old)
		} else {
			_ = db.DeleteTemplate(item.Name)
		}
		return fmt.Errorf("plantilla rechazada, cambio revertido: %w", err)
	}
	return nil
}
func (a *adminController) DeleteTemplate(_ context.Context, name string) error {
	if !validAdminServiceName(name) {
		return fmt.Errorf("nombre inválido")
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
	if settings["services.default_template"] == name {
		return fmt.Errorf("plantilla predeterminada: cambie primero el ajuste global")
	}
	services, err := db.ListServices()
	if err != nil {
		return err
	}
	for _, svc := range services {
		if svc.Template == name {
			return fmt.Errorf("la plantilla está utilizada por %s", svc.Name)
		}
	}
	templates, err := db.ListTemplates()
	if err != nil {
		return err
	}
	found := false
	for _, v := range templates {
		if v.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("plantilla inexistente")
	}
	if err = db.DeleteTemplate(name); err != nil {
		return err
	}
	return nil
}
func (a *adminController) reloadOrValidate() error {
	if a.runtime.active.Load() != nil {
		return a.runtime.reloadServicesLocked(a.configPath, a.vpn, a.provider)
	}
	candidate, err := loadConfig(a.configPath)
	if err != nil {
		return err
	}
	return validateStagedConfig(candidate)
}
