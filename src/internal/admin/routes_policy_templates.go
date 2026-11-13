package admin

import (
	"context"
	"net/http"

	"github.com/gentedelcondado/headscale-gateway/internal/policytemplates"
)

type PolicyTemplateController interface {
	PolicyTemplates(context.Context) ([]policytemplates.Template, error)
	StorePolicyTemplates(context.Context, policytemplates.Bundle, bool) (TemplateImportResult, error)
	DeletePolicyTemplate(context.Context, string) error
	PolicyRepoSettings(context.Context) (TemplateRepositorySettings, error)
	SavePolicyRepoSettings(context.Context, TemplateRepositorySettings) error
	FetchPolicyRepo(context.Context) (policytemplates.Bundle, error)
	SyncPolicyRepo(context.Context, bool) (TemplateImportResult, error)
}

func (s *Server) registerPolicyTemplateRoutes(mux routeRegistrar) {
	onlyAdmin := func(fn func(http.ResponseWriter, *http.Request, PolicyTemplateController)) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminRequest(r) {
				problem(w, 403, "solo administradores pueden gestionar plantillas ACL")
				return
			}
			c, ok := s.controller.(PolicyTemplateController)
			if !ok {
				problem(w, 501, "catálogo ACL no disponible")
				return
			}
			fn(w, r, c)
		})
	}
	mux.HandleFunc("GET /api/v1/network/admin/policy/templates", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		s.read(w, r, func(ctx context.Context) (any, error) { return c.PolicyTemplates(ctx) })
	}))
	mux.HandleFunc("POST /api/v1/network/admin/policy/templates/import", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		var request struct {
			policytemplates.Bundle
			Overwrite bool `json:"overwrite"`
		}
		if !decodeLimit(w, r, &request, policytemplates.MaxCatalogBytes) {
			return
		}
		result, err := c.StorePolicyTemplates(r.Context(), request.Bundle, request.Overwrite)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
	mux.HandleFunc("DELETE /api/v1/network/admin/policy/templates/{id}", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		if err := c.DeletePolicyTemplate(r.Context(), r.PathValue("id")); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/network/admin/policy/templates/export", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		templates, err := c.PolicyTemplates(r.Context())
		if err != nil {
			problem(w, 500, "no se pudo exportar el catálogo")
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="gateway-acl-templates.json"`)
		reply(w, 200, policytemplates.Bundle{Format: policytemplates.Format, Version: 1, Templates: templates}, nil)
	}))
	mux.HandleFunc("GET /api/v1/network/admin/policy/templates/repository", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		s.read(w, r, func(ctx context.Context) (any, error) { return c.PolicyRepoSettings(ctx) })
	}))
	mux.HandleFunc("PUT /api/v1/network/admin/policy/templates/repository", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		var request TemplateRepositorySettings
		if !decode(w, r, &request) {
			return
		}
		if err := c.SavePolicyRepoSettings(r.Context(), request); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/network/admin/policy/templates/repository/preview", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		result, err := c.FetchPolicyRepo(r.Context())
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
	mux.HandleFunc("POST /api/v1/network/admin/policy/templates/repository/sync", onlyAdmin(func(w http.ResponseWriter, r *http.Request, c PolicyTemplateController) {
		var req struct {
			Overwrite bool `json:"overwrite"`
		}
		if !decode(w, r, &req) {
			return
		}
		result, err := c.SyncPolicyRepo(r.Context(), req.Overwrite)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
}
