package admin

import (
	"context"
	"encoding/json"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"io"
	"net/http"
	"strings"
)

type portalController interface {
	PortalGet(context.Context) (any, error)
	PortalSave(context.Context, portal.Config) (any, error)
	PortalSync(context.Context) (any, error)
	PortalDiscover(context.Context) (any, error)
}

func (s *Server) registerPortalRoutes(mux routeRegistrar) {
	for _, operation := range []struct{ method, path string }{{"GET", ""}, {"PUT", ""}, {"POST", "/sync"}, {"POST", "/discover"}} {
		op := operation
		mux.HandleFunc(op.method+" /api/v1/portal"+op.path, s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.requirePKISession(w, r) {
				return
			}
			ctl, ok := s.controller.(portalController)
			if !ok {
				problem(w, 501, "portal no disponible")
				return
			}
			var result any
			var err error
			switch op.path {
			case "/sync":
				result, err = ctl.PortalSync(r.Context())
			case "/discover":
				result, err = ctl.PortalDiscover(r.Context())
			default:
				if op.method == "GET" {
					result, err = ctl.PortalGet(r.Context())
				} else {
					if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
						problem(w, 415, "Content-Type debe ser application/json")
						return
					}
					var cfg portal.Config
					decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
					decoder.DisallowUnknownFields()
					if e := decoder.Decode(&cfg); e != nil {
						problem(w, 400, "configuración de portal inválida")
						return
					}
					var extra any
					if decoder.Decode(&extra) != io.EOF {
						problem(w, 400, "solo se admite un objeto JSON")
						return
					}
					result, err = ctl.PortalSave(r.Context(), cfg)
				}
			}
			if err != nil {
				problem(w, 409, err.Error())
				return
			}
			reply(w, 200, result, nil)
		}))
	}
}
