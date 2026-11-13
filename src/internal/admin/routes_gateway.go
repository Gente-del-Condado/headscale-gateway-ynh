package admin

// Endpoints del Gateway: configuración, servicios, recarga y política de auditoría SQLite.
import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) registerGatewayRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v1/status", s.secure(func(w http.ResponseWriter, r *http.Request) { s.read(w, r, s.controller.Status) }))
	mux.HandleFunc("GET /api/v1/services", s.secure(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.controller.Services(r.Context())
		reply(w, http.StatusOK, v, e)
	}))
	mux.HandleFunc("GET /api/v1/templates", s.secure(func(w http.ResponseWriter, r *http.Request) {
		v, e := s.controller.Templates(r.Context())
		reply(w, http.StatusOK, v, e)
	}))
	// Las plantillas son objetos de la base de datos. Sus API exigen las mismas
	// comprobaciones de rol y módulo que la edición de servicios.
	mux.HandleFunc("GET /api/v1/templates/details", s.secure(func(w http.ResponseWriter, r *http.Request) {
		ctl, ok := s.controller.(interface {
			TemplateDetails(context.Context) ([]state.TemplateRecord, error)
		})
		if !ok {
			problem(w, 501, "plantillas no disponibles")
			return
		}
		result, err := ctl.TemplateDetails(r.Context())
		reply(w, 200, result, err)
	}))
	mux.HandleFunc("PUT /api/v1/templates/{name}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		ctl, ok := s.controller.(interface {
			SaveTemplate(context.Context, state.TemplateRecord) error
		})
		if !ok {
			problem(w, 501, "plantillas no disponibles")
			return
		}
		var body state.TemplateRecord
		if !decode(w, r, &body) {
			return
		}
		if body.Name != "" && body.Name != r.PathValue("name") {
			problem(w, 400, "nombre no coincide")
			return
		}
		body.Name = r.PathValue("name")
		if err := ctl.SaveTemplate(r.Context(), body); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"ok": true}, nil)
	}))
	mux.HandleFunc("DELETE /api/v1/templates/{name}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		ctl, ok := s.controller.(interface {
			DeleteTemplate(context.Context, string) error
		})
		if !ok {
			problem(w, 501, "plantillas no disponibles")
			return
		}
		if err := ctl.DeleteTemplate(r.Context(), r.PathValue("name")); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/proxy/schema", s.secure(func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, ProxySchema(), nil)
	}))
	mux.HandleFunc("GET /api/v1/config", s.secure(func(w http.ResponseWriter, r *http.Request) { s.read(w, r, s.controller.Settings) }))
	mux.HandleFunc("GET /api/v1/config/schema", s.secure(func(w http.ResponseWriter, r *http.Request) {
		fields, err := s.controller.ConfigSchema(r.Context())
		reply(w, http.StatusOK, fields, err)
	}))
	mux.HandleFunc("PUT /api/v1/config", s.secure(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Changes map[string]string `json:"changes"`
		}
		if !decode(w, r, &body) {
			return
		}
		if len(body.Changes) == 0 {
			problem(w, http.StatusBadRequest, "changes no puede estar vacío")
			return
		}
		if err := s.controller.UpdateSettings(r.Context(), body.Changes); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{"ok": true, "restart_required": true}, nil)
	}))
	mux.HandleFunc("POST /api/v1/services", s.secure(func(w http.ResponseWriter, r *http.Request) {
		var body Service
		if !decode(w, r, &body) {
			return
		}
		if body.Name == "" {
			problem(w, http.StatusBadRequest, "name obligatorio")
			return
		}
		if err := s.controller.SaveService(r.Context(), body); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{"ok": true}, nil)
	}))
	mux.HandleFunc("PUT /api/v1/services/{name}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		var body Service
		if !decode(w, r, &body) {
			return
		}
		name := r.PathValue("name")
		if body.Name != "" && body.Name != name {
			problem(w, http.StatusBadRequest, "el nombre no coincide con la ruta")
			return
		}
		body.Name = name
		if err := s.controller.SaveService(r.Context(), body); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{"ok": true}, nil)
	}))
	mux.HandleFunc("DELETE /api/v1/services/{name}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if err := s.controller.DeleteService(r.Context(), r.PathValue("name")); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{"ok": true}, nil)
	}))
	mux.HandleFunc("POST /api/v1/system/restart", s.secure(func(w http.ResponseWriter, r *http.Request) {
		controller, ok := s.controller.(RestartController)
		if !ok {
			problem(w, 501, "reinicio no disponible")
			return
		}
		if err := controller.ScheduleRestart(r.Context()); err != nil {
			problem(w, http.StatusConflict, err.Error())
			return
		}
		reply(w, http.StatusAccepted, map[string]any{"ok": true, "restarting": true}, nil)
	}))
	// Reintento ACME manual. El servidor respeta siempre el cooldown SQLite
	// y no expone ni claves privadas ni el token de Technitium/DNS.
	mux.HandleFunc("POST /api/v1/certificates/refresh", s.secure(func(w http.ResponseWriter, r *http.Request) {
		ctl, ok := s.controller.(interface {
			RefreshCertificate(context.Context) (any, error)
		})
		if !ok {
			problem(w, 501, "gestión de certificados no disponible")
			return
		}
		status, err := ctl.RefreshCertificate(r.Context())
		if err != nil {
			problem(w, 409, "certificado no actualizado; consulte logs y estado ACME")
			return
		}
		reply(w, 200, status, nil)
	}))
	mux.HandleFunc("POST /api/v1/reload", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if err := s.controller.Reload(r.Context()); err != nil {
			problem(w, http.StatusBadRequest, err.Error())
			return
		}
		reply(w, http.StatusOK, map[string]any{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/api/tokens", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.database == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		tokens, err := s.database.ListTokens()
		reply(w, 200, tokens, err)
	}))
	mux.HandleFunc("POST /api/v1/api/tokens", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.database == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		var req struct {
			Label  string   `json:"label"`
			Role   string   `json:"role"`
			Scopes []string `json:"scopes"`
			Days   int      `json:"days"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Role != "viewer" && req.Role != "operator" {
			problem(w, 400, "los tokens delegados solo pueden ser viewer u operator")
			return
		}
		if req.Days < 1 || req.Days > 365 || len(req.Scopes) == 0 {
			problem(w, 400, "caducidad o scopes no válidos")
			return
		}
		possible := map[string]bool{"read": true, "services": true, "network": true, "operations": true}
		for _, scope := range req.Scopes {
			method := http.MethodGet
			if scope == "operations" {
				method = http.MethodPost
			}
			if !possible[scope] || !rolePermits(req.Role, scope, method) {
				problem(w, 400, "módulo no válido para el rol")
				return
			}
		}
		token, info, err := s.database.NewToken(req.Label, req.Role, req.Scopes, time.Duration(req.Days)*24*time.Hour)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 201, map[string]any{"token": token, "info": info}, nil)
	}))
	mux.HandleFunc("DELETE /api/v1/api/tokens/{id}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.database == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		if err := s.database.RevokeToken(r.PathValue("id")); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/api/exposure", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.policy == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		reply(w, 200, s.policy.Get(), nil)
	}))
	mux.HandleFunc("PUT /api/v1/api/exposure", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.policy == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		var next APIPolicy
		if !decode(w, r, &next) {
			return
		}
		// Evita perder el acceso cuando Bearer es el único método de entrada configurado.
		if s.auth == nil || s.auth.Current().Mode == "token" {
			if !next.TokenIdentity || !next.TokenRead {
				problem(w, 400, "el modo token requiere mantener token_identity y token_read habilitados")
				return
			}
		}
		if err := s.policy.Update(next); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"ok": true, "policy": s.policy.Get()}, nil)
	}))
	mux.HandleFunc("GET /api/v1/audit", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if s.database == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		// Compatibilidad: los clientes antiguos reciben el formato anterior de array.
		if r.URL.RawQuery == "" {
			events, err := s.database.ListAudit(100)
			reply(w, 200, events, err)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		if page > 100000 {
			page = 100000
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size != 10 && size != 25 && size != 50 {
			size = 10
		}
		sortBy := strings.TrimSpace(r.URL.Query().Get("sort"))
		order := strings.TrimSpace(r.URL.Query().Get("order"))
		events, total, err := s.database.PageAudit(page, size, sortBy, order)
		reply(w, 200, map[string]any{"events": events, "total": total, "page": page, "size": size}, err)
	}))
}
