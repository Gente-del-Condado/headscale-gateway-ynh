package admin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"github.com/gentedelcondado/headscale-gateway/internal/templatelib"
)

type TemplateImportResult struct {
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}
type TemplateRepositorySettings struct {
	URL          string `json:"url"`
	AllowPrivate bool   `json:"allow_private"`
}
type TemplateController interface {
	ExportTemplates(context.Context) (templatelib.Bundle[state.TemplateRecord], error)
	ImportTemplates(context.Context, templatelib.Bundle[state.TemplateRecord], bool) (TemplateImportResult, error)
	RepositorySettings(context.Context) (TemplateRepositorySettings, error)
	SaveRepositorySettings(context.Context, TemplateRepositorySettings) error
	DownloadRepository(context.Context) (templatelib.Bundle[state.TemplateRecord], error)
	SyncRepository(context.Context, bool) (TemplateImportResult, error)
}

func (s *Server) registerTemplateTransferRoutes(mux routeRegistrar) {
	adminOnly := func(next func(http.ResponseWriter, *http.Request, TemplateController)) http.HandlerFunc {
		return s.secure(func(w http.ResponseWriter, r *http.Request) {
			if !s.isAdminRequest(r) {
				problem(w, 403, "solo un administrador puede gestionar repositorios e importaciones")
				return
			}
			ctl, ok := s.controller.(TemplateController)
			if !ok {
				problem(w, 501, "transferencia de plantillas no disponible")
				return
			}
			next(w, r, ctl)
		})
	}
	mux.HandleFunc("GET /api/v1/templates/export", adminOnly(func(w http.ResponseWriter, r *http.Request, c TemplateController) {
		bundle, err := c.ExportTemplates(r.Context())
		if err != nil {
			problem(w, 500, "no se pudieron exportar las plantillas")
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="headscale-gateway-templates.json"`)
		reply(w, 200, bundle, nil)
	}))
	mux.HandleFunc("POST /api/v1/templates/import", adminOnly(func(w http.ResponseWriter, r *http.Request, c TemplateController) {
		var request struct {
			Format    string                 `json:"format"`
			Version   int                    `json:"version"`
			Templates []state.TemplateRecord `json:"templates"`
			Overwrite bool                   `json:"overwrite"`
		}
		if !decodeLimit(w, r, &request, templatelib.MaxBytes) {
			return
		}
		result, err := c.ImportTemplates(r.Context(), templatelib.Bundle[state.TemplateRecord]{Format: request.Format, Version: request.Version, Templates: request.Templates}, request.Overwrite)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
	mux.HandleFunc("GET /api/v1/templates/repository", adminOnly(func(w http.ResponseWriter, r *http.Request, c TemplateController) {
		s.read(w, r, func(ctx context.Context) (any, error) { return c.RepositorySettings(ctx) })
	}))
	mux.HandleFunc("PUT /api/v1/templates/repository", adminOnly(func(w http.ResponseWriter, r *http.Request, c TemplateController) {
		var settings TemplateRepositorySettings
		if !decode(w, r, &settings) {
			return
		}
		if err := c.SaveRepositorySettings(r.Context(), settings); err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, map[string]bool{"ok": true}, nil)
	}))
	mux.HandleFunc("GET /api/v1/templates/repository/preview", adminOnly(func(w http.ResponseWriter, r *http.Request, c TemplateController) {
		result, err := c.DownloadRepository(r.Context())
		if err != nil {
			problem(w, 502, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
	mux.HandleFunc("POST /api/v1/templates/repository/sync", adminOnly(func(w http.ResponseWriter, r *http.Request, c TemplateController) {
		var req struct {
			Overwrite bool `json:"overwrite"`
		}
		if !decode(w, r, &req) {
			return
		}
		result, err := c.SyncRepository(r.Context(), req.Overwrite)
		if err != nil {
			problem(w, 400, err.Error())
			return
		}
		reply(w, 200, result, nil)
	}))
}
func decodeLimit(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		problem(w, 415, "Content-Type debe ser application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		problem(w, 400, "JSON de importación inválido")
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err == io.EOF {
		return true
	}
	problem(w, 400, "solo se admite un objeto JSON")
	return false
}
