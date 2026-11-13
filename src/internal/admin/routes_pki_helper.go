package admin

import (
	"context"
	"errors"
	"github.com/gentedelcondado/headscale-gateway/internal/pki"
	"net/http"
)

const caHelperKey = "pki.interactive_helper.v1"

func (s *Server) helperEnabled() bool {
	if pki.SystemPlatform() == "android" {
		return false
	}
	var enabled bool
	if s.database != nil {
		_, err := s.database.GetJSON(caHelperKey, &enabled)
		if err != nil {
			return false
		}
	}
	return enabled
}
func (s *Server) changeSystemCA(ctx context.Context, action string, a pki.Anchor) error {
	if s.helperEnabled() {
		return pki.RunElevatedHelper(ctx, action, a)
	}
	if action == "install" {
		return pki.Install(ctx, a)
	}
	if action == "uninstall" {
		return pki.Uninstall(ctx, a)
	}
	return errors.New("operación no permitida")
}
func (s *Server) registerCAHelperRoutes(mux routeRegistrar) {
	mux.HandleFunc("PUT /api/v1/pki/helper", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !decode(w, r, &req) {
			return
		}
		if s.database == nil {
			problem(w, 503, "base de datos no disponible")
			return
		}
		if req.Enabled && pki.HelperStatus()["available"] != true {
			problem(w, 409, "coloca el helper de este sistema junto al ejecutable del Gateway")
			return
		}
		if err := s.database.PutJSON(caHelperKey, req.Enabled); err != nil {
			problem(w, 500, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"enabled": req.Enabled}, nil)
	}))
}
