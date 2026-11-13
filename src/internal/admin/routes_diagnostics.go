package admin

import (
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"
	"net/http"
	"strconv"
)

type proxyConnectionCleaner interface{ ClearProxyConnections() }

// registerDiagnosticsRoutes nunca expone una ruta elegida por el cliente.
// Los registros pueden revelar infraestructura; solo los administradores los leen.
func (s *Server) registerDiagnosticsRoutes(mux routeRegistrar) {
	mux.HandleFunc("POST /api/v1/diagnostics/logs/clear", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) {
			problem(w, 403, "solo administrador")
			return
		}
		var body struct {
			Confirm string `json:"confirm"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Confirm != "LIMPIAR" {
			problem(w, 400, "confirma LIMPIAR")
			return
		}
		if err := diagnostics.Clear(); err != nil {
			problem(w, 500, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"cleared": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/diagnostics/logs", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) {
			problem(w, 403, "solo administrador")
			return
		}
		reply(w, 200, diagnostics.Info(), nil)
	}))
	mux.HandleFunc("GET /api/v1/diagnostics/logs/{channel}", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) {
			problem(w, 403, "solo administrador")
			return
		}
		count, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if count <= 0 {
			count = 100
		}
		lines, err := diagnostics.Tail(r.PathValue("channel"), count)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"channel": r.PathValue("channel"), "lines": lines}, nil)
	}))
	mux.HandleFunc("PUT /api/v1/diagnostics/logs/settings", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) {
			problem(w, 403, "solo administrador")
			return
		}
		if s.database == nil {
			problem(w, 503, "SQLite no disponible")
			return
		}
		var config struct {
			Directory string `json:"directory"`
			MaxMB     int    `json:"max_mb"`
			Backups   int    `json:"backups"`
			Level     string `json:"level"`
		}
		if !decode(w, r, &config) {
			return
		}
		if err := diagnostics.ValidateDir(config.Directory); err != nil {
			problem(w, 400, err.Error())
			return
		}
		if config.Level == "" {
			config.Level = "info"
		}
		if _, err := diagnostics.ParseLevel(config.Level); err != nil {
			problem(w, 400, err.Error())
			return
		}
		original := diagnostics.Info()
		if err := diagnostics.Configure(config.Directory, config.MaxMB, config.Backups); err != nil {
			problem(w, 400, err.Error())
			return
		}
		if err := s.database.PutJSON("diagnostics.logging.v1", config); err != nil {
			_ = diagnostics.Configure(original.Directory, original.MaxMB, original.Backups)
			problem(w, 503, err.Error())
			return
		}
		_ = diagnostics.SetLevel(config.Level)
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-ADMIN-LOG-002", "parámetros de registro actualizados; nivel=%s", diagnostics.EffectiveLevel().Label())
		reply(w, 200, diagnostics.Info(), nil)
	}))
	mux.HandleFunc("POST /api/v1/diagnostics/logs/debug", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) {
			problem(w, 403, "solo administrador")
			return
		}
		var body struct {
			Minutes int `json:"minutes"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Minutes == 0 {
			diagnostics.DisableTemporaryDebug()
		} else if _, err := diagnostics.EnableTemporaryDebug(body.Minutes); err != nil {
			problem(w, 400, err.Error())
			return
		}
		diagnostics.Emit("admin", diagnostics.InfoLevel, "HGW-DEBUG-001", "depuración temporal configurada; minutos=%d", body.Minutes)
		reply(w, 200, diagnostics.Info(), nil)
	}))
	mux.HandleFunc("POST /api/v1/diagnostics/proxy/clear-idle", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) {
			problem(w, 403, "solo administrador")
			return
		}
		if ctl, ok := s.controller.(proxyConnectionCleaner); ok {
			ctl.ClearProxyConnections()
		}
		diagnostics.Emit("proxy", diagnostics.InfoLevel, "HGW-PROXY-CONN-003", "conexiones inactivas cerradas por administración")
		reply(w, 200, map[string]any{"ok": true, "response_cache": false}, nil)
	}))
}
