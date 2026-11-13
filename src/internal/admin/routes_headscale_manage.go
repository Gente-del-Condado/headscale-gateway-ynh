package admin

import (
	"context"
	"net/http"
)

// HeadscaleAction solo acepta comandos de la lista permitida del Gateway,
// nunca URL ni métodos HTTP arbitrarios elegidos por el navegador.
type HeadscaleAction struct {
	Action    string   `json:"action"`
	ID        string   `json:"id,omitempty"`
	Name      string   `json:"name,omitempty"`
	User      string   `json:"user,omitempty"`
	Routes    []string `json:"routes,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Reusable  bool     `json:"reusable,omitempty"`
	Ephemeral bool     `json:"ephemeral,omitempty"`
	Hours     int      `json:"hours,omitempty"`
}
type HeadscaleManagementController interface {
	HeadscaleUsers(context.Context) (any, error)
	HeadscalePreAuthKeys(context.Context) (any, error)
	HeadscaleManage(context.Context, HeadscaleAction) (any, error)
}

func (s *Server) registerHeadscaleManagementRoutes(mux routeRegistrar) {
	onlyAdmin := func(fn func(http.ResponseWriter, *http.Request, HeadscaleManagementController)) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminRequest(r) {
				problem(w, http.StatusForbidden, "solo los administradores pueden modificar Headscale")
				return
			}
			c, ok := s.controller.(HeadscaleManagementController)
			if !ok {
				problem(w, http.StatusNotImplemented, "administración de Headscale no disponible")
				return
			}
			fn(w, r, c)
		})
	}
	mux.HandleFunc("GET /api/v1/network/admin/users", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c HeadscaleManagementController) {
		data, err := c.HeadscaleUsers(r.Context())
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 200, data, nil)
	}))
	mux.HandleFunc("GET /api/v1/network/admin/keys", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c HeadscaleManagementController) {
		data, err := c.HeadscalePreAuthKeys(r.Context())
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 200, data, nil)
	}))
	mux.HandleFunc("POST /api/v1/network/admin/action", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c HeadscaleManagementController) {
		var action HeadscaleAction
		if !decode(w, r, &action) {
			return
		}
		data, err := c.HeadscaleManage(r.Context(), action)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, data, nil)
	}))
}
