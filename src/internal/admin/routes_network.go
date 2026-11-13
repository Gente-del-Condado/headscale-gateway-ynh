package admin

import (
	"context"
	"errors"
	"net/http"
)

// Las rutas de red son opcionales: el Gateway implementa NetworkController.
func (s *Server) registerNetworkRoutes(mux routeRegistrar) {
	// Endpoints opcionales: el controlador real implementa NetworkController.
	networkHandler := func(action func(NetworkController, context.Context, *http.Request, http.ResponseWriter) (any, error)) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			nc, ok := s.controller.(NetworkController)
			if !ok {
				problem(w, 404, "panel de red no disponible")
				return
			}
			value, err := action(nc, r.Context(), r, w)
			if err != nil {
				if errors.Is(err, ErrAlreadyReplied) {
					return
				}
				if errors.Is(err, ErrNetworkDisabled) {
					problem(w, 404, "panel de red desactivado")
					return
				}
				problem(w, 502, err.Error())
				return
			}
			reply(w, 200, value, nil)
		})
	}
	mux.HandleFunc("GET /api/v1/network/info", networkHandler(func(c NetworkController, ctx context.Context, r *http.Request, w http.ResponseWriter) (any, error) {
		return c.NetworkInfo(ctx)
	}))
	mux.HandleFunc("GET /api/v1/network/nodes", networkHandler(func(c NetworkController, ctx context.Context, r *http.Request, w http.ResponseWriter) (any, error) {
		return c.NetworkNodes(ctx)
	}))
	mux.HandleFunc("POST /api/v1/network/ping", networkHandler(func(c NetworkController, ctx context.Context, r *http.Request, w http.ResponseWriter) (any, error) {
		var req struct {
			NodeID string `json:"node_id"`
			Mode   string `json:"mode"`
		}
		if !decode(w, r, &req) {
			return nil, ErrAlreadyReplied
		}
		return c.NetworkPing(ctx, req.NodeID, req.Mode)
	}))
	mux.HandleFunc("POST /api/v1/network/login", networkHandler(func(c NetworkController, ctx context.Context, r *http.Request, w http.ResponseWriter) (any, error) {
		return c.NetworkLogin(ctx)
	}))

}
