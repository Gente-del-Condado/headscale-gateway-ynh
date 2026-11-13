package admin

import (
	"context"
	"net/http"
	"strings"
)

type GatewayPrefsRequest struct {
	AcceptRoutes           *bool     `json:"accept_routes,omitempty"`
	AcceptDNS              *bool     `json:"accept_dns,omitempty"`
	ExitNodeAllowLANAccess *bool     `json:"exit_node_allow_lan_access,omitempty"`
	AdvertiseRoutes        *[]string `json:"advertise_routes,omitempty"`
}

func (s *Server) registerGatewayPrefsRoutes(mux routeRegistrar) {
	mux.HandleFunc("POST /api/v1/network/disconnect", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) || s.auth == nil || !s.auth.sessionAllowed(r) || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			problem(w, 403, "requiere sesión interactiva admin")
			return
		}
		ctl, ok := s.controller.(interface {
			NetworkDisconnect(context.Context) (any, error)
		})
		if !ok {
			problem(w, 501, "desconexión no disponible")
			return
		}
		result, err := ctl.NetworkDisconnect(r.Context())
		if err != nil {
			problem(w, 409, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))

	mux.HandleFunc("PUT /api/v1/network/gateway/preferences", s.secure(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdminRequest(r) || s.auth == nil || !s.auth.sessionAllowed(r) || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			problem(w, 403, "requiere sesión interactiva admin")
			return
		}
		ctl, ok := s.controller.(interface {
			GatewayPrefsChange(context.Context, GatewayPrefsRequest) (any, error)
		})
		if !ok {
			problem(w, 501, "preferencias tsnet no disponibles")
			return
		}
		var input GatewayPrefsRequest
		if !decode(w, r, &input) {
			return
		}
		response, e := ctl.GatewayPrefsChange(r.Context(), input)
		if e != nil {
			problem(w, 409, e.Error())
			return
		}
		reply(w, 200, response, nil)
	}))
}
