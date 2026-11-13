package admin

import (
	"context"
	"encoding/base64"
	"net/http"
)

type configurationTransfer interface {
	ExportConfiguration(context.Context, bool, string) ([]byte, error)
	ImportConfiguration(context.Context, []byte, string) (int, error)
}

// Estas rutas exigen sesión interactiva admin además de la protección CSRF
// existente. Los tokens Bearer (incluida la credencial de recuperación) no
// pueden exportar los secretos de configuración.
func (s *Server) registerConfigurationTransferRoutes(mux routeRegistrar) {
	gate := func(fn http.HandlerFunc) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.requirePKISession(w, r) {
				return
			}
			fn(w, r)
		})
	}
	mux.HandleFunc("POST /api/v1/config/transfer/export", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
			Secrets  bool   `json:"secrets"`
		}
		if !decodeLimit(w, r, &body, 16<<10) {
			return
		}
		controller, ok := s.controller.(configurationTransfer)
		if !ok {
			problem(w, 501, "exportación no disponible")
			return
		}
		data, err := controller.ExportConfiguration(r.Context(), body.Secrets, body.Password)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"filename": "headscale-gateway-config.hgwcfg", "data": base64.StdEncoding.EncodeToString(data)}, nil)
	}))
	mux.HandleFunc("POST /api/v1/config/transfer/import", gate(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Password string `json:"password"`
			Data     string `json:"data"`
		}
		if !decodeLimit(w, r, &body, 520<<10) {
			return
		}
		data, err := base64.StdEncoding.DecodeString(body.Data)
		if err != nil || len(data) > 320<<10 {
			problem(w, 400, "archivo cifrado inválido o demasiado grande")
			return
		}
		controller, ok := s.controller.(configurationTransfer)
		if !ok {
			problem(w, 501, "importación no disponible")
			return
		}
		count, err := controller.ImportConfiguration(r.Context(), data, body.Password)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]any{"imported": count, "restart_required": true, "preserved_local_access": true}, nil)
	}))
}
