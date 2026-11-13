package admin

import (
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net/http"
	"strings"
	"sync"
)

type APIPolicy struct {
	TokenRead       bool `json:"token_read"`
	TokenServices   bool `json:"token_services"`
	TokenSettings   bool `json:"token_settings"`
	TokenNetwork    bool `json:"token_network"`
	TokenOperations bool `json:"token_operations"`
	TokenIdentity   bool `json:"token_identity"`
}

func DefaultAPIPolicy() APIPolicy { return APIPolicy{true, true, true, true, true, true} }

type APIPolicyManager struct {
	mu     sync.RWMutex
	db     *state.Store
	policy APIPolicy
}

func NewAPIPolicyManager(db *state.Store) (*APIPolicyManager, error) {
	if db == nil {
		return nil, errors.New("SQLite requerido")
	}
	p := &APIPolicyManager{db: db, policy: DefaultAPIPolicy()}
	var current APIPolicy
	ok, e := db.GetJSON("api_exposure_v1", &current)
	if e != nil {
		return nil, e
	}
	if ok {
		p.policy = current
	}
	return p, nil
}
func (m *APIPolicyManager) Get() APIPolicy { m.mu.RLock(); defer m.mu.RUnlock(); return m.policy }
func (m *APIPolicyManager) Update(next APIPolicy) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.db.PutJSON("api_exposure_v1", next); err != nil {
		return err
	}
	m.policy = next
	return nil
}
func (m *APIPolicyManager) TokenAllowed(cat string) bool {
	p := m.Get()
	switch cat {
	case "read":
		return p.TokenRead
	case "services":
		return p.TokenServices
	case "settings":
		return p.TokenSettings
	case "network":
		return p.TokenNetwork
	case "operations":
		return p.TokenOperations
	case "identity":
		return p.TokenIdentity
	default:
		return false
	}
}
func routeCategory(r *http.Request) string {
	path := r.URL.Path
	switch {
	case strings.HasPrefix(path, "/api/v1/reference/peer/"):
		return "settings"
	case path == "/api/v1/auth/session" || path == "/api/v1/auth/logout":
		return "read"
	case strings.HasPrefix(path, "/api/v1/pki"):
		return "identity"
	case strings.HasPrefix(path, "/api/v1/portal"):
		return "settings"
	case strings.HasPrefix(path, "/api/v1/backups"), strings.HasPrefix(path, "/api/v1/forgejo/"):
		return "settings"
	case strings.HasPrefix(path, "/api/v1/diagnostics/"):
		return "settings"
	case strings.HasPrefix(path, "/api/v1/auth/") || strings.HasPrefix(path, "/api/v1/api/") || strings.HasPrefix(path, "/api/v1/audit"):
		return "identity"
	case strings.HasPrefix(path, "/api/v1/services") || strings.HasPrefix(path, "/api/v1/templates") || strings.HasPrefix(path, "/api/v1/proxy/"):
		return "services"
	case strings.HasPrefix(path, "/api/v1/config"):
		return "settings"
	case strings.HasPrefix(path, "/api/v1/network"):
		return "network"
	case strings.HasPrefix(path, "/api/v1/certificates/"):
		return "settings" // la renovación manual requiere administrador
	case strings.HasPrefix(path, "/api/v1/system/"):
		return "settings" // reinicio solo administradores
	case path == "/api/v1/reload":
		return "operations"
	default:
		return "read"
	}
}
func rolePermits(role, category, method string) bool {
	if role == "admin" {
		return true
	}
	if role == "viewer" {
		if category == "read" && method == http.MethodPost {
			return true
		} // cerrar sesión
		return method == http.MethodGet && (category == "read" || category == "services" || category == "network")
	}
	if role == "operator" {
		return (category == "read" || category == "services" || category == "network" || category == "operations") && (category != "operations" || method == http.MethodPost)
	}
	return false
}

// Los grupos no distinguen mayúsculas y solo se asignan a roles conocidos.
func roleForGroups(mapping map[string]string, groups, allowed []string) string {
	if len(mapping) == 0 {
		// Autenticar no concede permisos: sin una asignación explícita de grupo y rol,
		// ninguna identidad LDAP u OIDC recibe administración.
		return ""
	}
	matched := ""
	for _, g := range groups {
		if len(allowed) > 0 && !containsIdentity(allowed, g) {
			continue
		}
		for k, role := range mapping {
			if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(g)) {
				if role == "admin" {
					return "admin"
				}
				if role == "operator" {
					matched = "operator"
				} else if role == "viewer" && matched == "" {
					matched = "viewer"
				}
			}
		}
	}
	return matched
}

// Un rol explícito por correo verificado prevalece sobre el del grupo. Admite
// Google, que no envía grupos, sin conceder administración automáticamente.
// El llamante debe verificar antes la firma del ID token OIDC y email_verified.
func roleForOIDC(groups, emails map[string]string, actualGroups, allowedGroups []string, verifiedEmail string) string {
	for email, role := range emails {
		if containsIdentity([]string{email}, verifiedEmail) {
			return role
		}
	}
	return roleForGroups(groups, actualGroups, allowedGroups)
}
