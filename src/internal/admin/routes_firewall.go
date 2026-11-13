package admin

import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/platform/firewall"
	"net/http"
)

func (s *Server) registerFirewallRoutes(mux routeRegistrar) {
	mux.HandleFunc("GET /api/v1/system/firewall", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.requirePKISession(w, r) {
			return
		}
		ctl, ok := s.controller.(interface {
			FirewallReport(context.Context) (firewall.Report, error)
		})
		if !ok {
			problem(w, 501, "diagnóstico de cortafuegos no disponible")
			return
		}
		result, err := ctl.FirewallReport(r.Context())
		reply(w, 200, result, err)
	}))
}
