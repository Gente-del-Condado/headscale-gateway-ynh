package admin

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// routeRegistrar registra las mismas rutas que net/http, incluidas las generadas.
// Las pruebas pueden seguir proporcionando un ServeMux normal.
type routeRegistrar interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}
type referenceMux struct {
	*http.ServeMux
	entries []referenceEntry
}
type referenceEntry struct {
	Method string   `json:"method"`
	Path   string   `json:"path"`
	Group  string   `json:"group"`
	Scope  string   `json:"scope"`
	Roles  []string `json:"roles"`
	Auth   string   `json:"auth"`
	Source string   `json:"source"`
}

func (m *referenceMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.ServeMux.HandleFunc(pattern, handler)
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || !(strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/pki/") || strings.HasPrefix(path, "/admin/auth/")) {
		return
	}
	_, file, _, _ := runtime.Caller(1)
	req, _ := http.NewRequest(method, path, nil)
	scope := routeCategory(req)
	group := strings.TrimPrefix(path, "/api/v1/")
	group = strings.Split(group, "/")[0]
	if strings.HasPrefix(path, "/pki/") {
		group = "federation"
	}
	if strings.HasPrefix(path, "/admin/auth/") {
		group = "auth"
	}
	auth := "session_or_token"
	if strings.HasPrefix(path, "/api/v1/backups") || strings.HasPrefix(path, "/api/v1/portal") || strings.HasPrefix(path, "/api/v1/reference/peer/") || strings.HasPrefix(path, "/api/v1/config/transfer") || strings.Contains(path, "/mfa/") || strings.HasSuffix(path, "/recovery/rotate") || strings.HasSuffix(path, "/firewall") || strings.HasPrefix(path, "/api/v1/pki/") && !strings.HasSuffix(path, "/certificate") {
		auth = "admin_session"
	}
	if path == "/api/v1/network/disconnect" || path == "/api/v1/network/gateway/preferences" {
		auth = "admin_session"
	}
	if path == "/api/v1/auth/methods" || path == "/pki/v1/public-ca" {
		auth = "public"
	}
	if path == "/api/v1/auth/login" || path == "/api/v1/auth/bootstrap" || strings.HasPrefix(path, "/admin/auth/") {
		auth = "login_flow"
	}
	roles := []string{}
	for _, role := range []string{"viewer", "operator", "admin"} {
		if rolePermits(role, scope, method) && (auth != "admin_session" || role == "admin") && (!strings.HasPrefix(path, "/api/v1/network/admin/") || role == "admin") {
			roles = append(roles, role)
		}
	}
	if auth == "public" || auth == "login_flow" {
		roles = []string{}
		scope = ""
	}
	m.entries = append(m.entries, referenceEntry{method, path, group, scope, roles, auth, "internal/admin/" + filepath.Base(file)})
}

//go:embed reference-data.json
var referenceData embed.FS

func (s *Server) registerReferenceRoutes(mux *referenceMux) {
	mux.HandleFunc("GET /api/v1/reference", s.secure(func(w http.ResponseWriter, r *http.Request) {
		data, _ := referenceData.ReadFile("reference-data.json")
		var extra map[string]any
		_ = json.Unmarshal(data, &extra)
		rows := append([]referenceEntry(nil), mux.entries...)
		rows = append(rows, referenceEntry{Method: "GET", Path: "/.well-known/headscale-gateway", Group: "federation", Auth: "public_opt_in", Source: "portal_runtime.go", Roles: []string{}})
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Path == rows[j].Path {
				return rows[i].Method < rows[j].Method
			}
			return rows[i].Path < rows[j].Path
		})
		extra["routes"] = rows
		reply(w, 200, extra, nil)
	}))
	mux.HandleFunc("GET /api/v1/reference/peer/{fingerprint}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		ctl, ok := s.controller.(interface {
			PortalProbe(context.Context, string) (any, error)
		})
		if !ok {
			problem(w, 501, "portal no disponible")
			return
		}
		result, err := ctl.PortalProbe(r.Context(), r.PathValue("fingerprint"))
		if err != nil {
			problem(w, 409, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
}
