package main

// Operaciones CRUD para servicios publicados en SQLite.
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/admin"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"sort"
	"strings"
)

func (a *adminController) Services(_ context.Context) ([]admin.Service, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.ListServices()
	if err != nil {
		return nil, err
	}
	result := make([]admin.Service, 0, len(rows))
	for _, v := range rows {
		enabled := v.Enabled
		result = append(result, admin.Service{Name: v.Name, Description: v.Description, Enabled: &enabled, Backend: v.Backend, Template: v.Template, Proxy: v.Proxy})
	}
	return result, nil
}
func (a *adminController) Templates(_ context.Context) ([]string, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.ListTemplates()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, v := range rows {
		names = append(names, v.Name)
	}
	sort.Strings(names)
	return names, nil
}

// La configuración nunca revela las credenciales. Los campos secretos vacíos significan «mantener los valores existentes».
func validAdminServiceName(name string) bool {
	if name == "" || len(name) > 63 || normalizeServiceName(name) != name {
		return false
	}
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		if r == '-' && i > 0 && i < len(name)-1 {
			continue
		}
		return false
	}
	return true
}

// SaveService valida una generación candidata antes de conservar el estado final
// de la transacción. En caso de fallo, restaura la fila anterior y la generación activa.
func (a *adminController) SaveService(_ context.Context, item admin.Service) error {
	if !validAdminServiceName(item.Name) {
		return fmt.Errorf("nombre de servicio inválido: %q", item.Name)
	}
	if len(item.Proxy) > 32768 {
		return fmt.Errorf("proxy demasiado grande")
	}
	if len(item.Proxy) > 0 && !json.Valid(item.Proxy) {
		return fmt.Errorf("JSON de proxy inválido")
	}
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	db, err := openCatalog(a.configPath)
	if err != nil {
		return err
	}
	defer db.Close()
	oldRows, err := db.ListServices()
	if err != nil {
		return err
	}
	var old state.ServiceRecord
	existed := false
	for _, v := range oldRows {
		if v.Name == item.Name {
			old = v
			existed = true
			break
		}
	}
	enabled := true
	if existed {
		enabled = old.Enabled
	}
	if item.Enabled != nil {
		enabled = *item.Enabled
	}
	record := state.ServiceRecord{Name: item.Name, Description: strings.TrimSpace(item.Description), Enabled: enabled, Backend: strings.TrimSpace(item.Backend), Template: item.Template, Proxy: item.Proxy}
	if item.Proxy == nil && existed {
		record.Proxy = old.Proxy
	}
	if err = db.SaveService(record); err != nil {
		return err
	}
	return a.checkAndRollbackService(db, old, existed, item.Name)
}
func (a *adminController) DeleteService(_ context.Context, name string) error {
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
	rows, err := db.ListServices()
	if err != nil {
		return err
	}
	var old state.ServiceRecord
	found := false
	for _, v := range rows {
		if v.Name == name {
			old = v
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("servicio no encontrado")
	}
	if err = db.DeleteService(name); err != nil {
		return err
	}
	return a.checkAndRollbackService(db, old, true, name)
}
func (a *adminController) checkAndRollbackService(db *state.Store, old state.ServiceRecord, existed bool, name string) error {
	var err error
	if a.runtime.active.Load() != nil {
		err = a.runtime.reloadServicesLocked(a.configPath, a.vpn, a.provider)
	} else {
		var candidate Config
		candidate, err = loadConfig(a.configPath)
		if err == nil {
			err = validateStagedConfig(candidate)
		}
	}
	if err == nil {
		return nil
	}
	var rollback error
	if existed {
		rollback = db.SaveService(old)
	} else {
		rollback = db.DeleteService(name)
	}
	if rollback != nil {
		return fmt.Errorf("recarga fallida (%v); rollback SQL falló: %w", err, rollback)
	}
	return fmt.Errorf("cambios no aplicados, catálogo restaurado: %w", err)
}
func (a *adminController) Reload(_ context.Context) error {
	if a.runtime.active.Load() == nil {
		return fmt.Errorf("modo configuración: guarda y reinicia")
	}
	return a.runtime.reloadServices(a.configPath, a.vpn, a.provider)
}

//La gestión de plantillas coexiste con la gestión de servicios, pero utiliza filas de SQLite independientes.
//No se permite eliminar una plantilla si esta es referenciada por un servicio o si se trata de la plantilla predeterminada.
//los servicios existentes nunca modifican su comportamiento de forma silenciosa.
