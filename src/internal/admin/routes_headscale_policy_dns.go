package admin

import (
	"context"
	"github.com/gentedelcondado/headscale-gateway/internal/network"
	"github.com/gentedelcondado/headscale-gateway/internal/policytemplates"
	"net/http"
)

type HeadscalePolicyDNSController interface {
	HeadscaleGetPolicy(context.Context) (network.PolicyDocument, error)
	HeadscaleSetPolicy(context.Context, string, string) (network.PolicyDocument, error)
	HeadscaleServiceDNS(context.Context) ([]network.MagicDNSRecord, error)
}

// Endpoints de lectura y escritura solo para administradores; no son un proxy Headscale genérico.
func (s *Server) registerHeadscalePolicyDNSRoutes(mux routeRegistrar) {
	gate := func(fn func(http.ResponseWriter, *http.Request, HeadscalePolicyDNSController)) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminRequest(r) {
				problem(w, 403, "acceso reservado al administrador")
				return
			}
			ctl, ok := s.controller.(HeadscalePolicyDNSController)
			if !ok {
				problem(w, 501, "gestión de políticas no disponible")
				return
			}
			fn(w, r, ctl)
		})
	}
	mux.HandleFunc("GET /api/v1/network/admin/policy", gate(func(w http.ResponseWriter, r *http.Request, ctl HeadscalePolicyDNSController) {
		result, err := ctl.HeadscaleGetPolicy(r.Context())
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
	mux.HandleFunc("PUT /api/v1/network/admin/policy", gate(func(w http.ResponseWriter, r *http.Request, ctl HeadscalePolicyDNSController) {
		var data struct {
			Policy   string `json:"policy"`
			Revision string `json:"revision"`
		}
		if !decode(w, r, &data) {
			return
		}
		result, err := ctl.HeadscaleSetPolicy(r.Context(), data.Policy, data.Revision)
		if err != nil {
			problem(w, 409, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))

	mux.HandleFunc("POST /api/v1/network/admin/policy/parse", gate(func(w http.ResponseWriter, r *http.Request, ctl HeadscalePolicyDNSController) {
		var body struct {
			Policy string `json:"policy"`
		}
		if !decodeLimit(w, r, &body, 100*1024+1024) {
			return
		}
		parsed, err := policytemplates.Normalize(body.Policy)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, parsed, nil)
	}))
	mux.HandleFunc("GET /api/v1/network/admin/dns/records", gate(func(w http.ResponseWriter, r *http.Request, ctl HeadscalePolicyDNSController) {
		result, err := ctl.HeadscaleServiceDNS(r.Context())
		if err != nil {
			problem(w, 409, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
}
