package admin

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"github.com/gentedelcondado/headscale-gateway/internal/templatelib"
)

type transferTestController struct {
	fakeController
	imported    int
	overwritten bool
}

func (*transferTestController) ExportTemplates(context.Context) (templatelib.Bundle[state.TemplateRecord], error) {
	return templatelib.Bundle[state.TemplateRecord]{Format: templatelib.Format, Version: 1, Templates: []state.TemplateRecord{{Name: "web", Proxy: json.RawMessage(`{}`)}}}, nil
}
func (c *transferTestController) ImportTemplates(_ context.Context, b templatelib.Bundle[state.TemplateRecord], over bool) (TemplateImportResult, error) {
	c.imported = len(b.Templates)
	c.overwritten = over
	return TemplateImportResult{Added: c.imported}, nil
}
func (*transferTestController) RepositorySettings(context.Context) (TemplateRepositorySettings, error) {
	return TemplateRepositorySettings{URL: "https://example.test/plantillas.json"}, nil
}
func (*transferTestController) SaveRepositorySettings(context.Context, TemplateRepositorySettings) error {
	return nil
}
func (*transferTestController) DownloadRepository(context.Context) (templatelib.Bundle[state.TemplateRecord], error) {
	return templatelib.Bundle[state.TemplateRecord]{Format: templatelib.Format, Version: 1, Templates: []state.TemplateRecord{{Name: "web", Proxy: json.RawMessage(`{}`)}}}, nil
}
func (*transferTestController) SyncRepository(context.Context, bool) (TemplateImportResult, error) {
	return TemplateImportResult{Updated: 1}, nil
}

func TestTemplateTransferAPI(t *testing.T) {
	controller := &transferTestController{}
	server, err := New(strings.Repeat("a", 64), controller)
	if err != nil {
		t.Fatal(err)
	}
	h := server.Handler()
	call := func(method, path, body, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if auth != "" {
			req.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	if got := call("GET", "/api/v1/templates/export", "", "").Code; got != 401 {
		t.Fatalf("export sin auth: %d", got)
	}
	w := call("GET", "/api/v1/templates/export", "", strings.Repeat("a", 64))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"format":"headscale-gateway-templates"`) {
		t.Fatalf("export: %d %s", w.Code, w.Body.String())
	}
	w = call("POST", "/api/v1/templates/import", `{"format":"headscale-gateway-templates","version":1,"templates":[{"name":"web","proxy":{}}],"overwrite":true}`, strings.Repeat("a", 64))
	if w.Code != 200 || controller.imported != 1 || !controller.overwritten {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
	w = call("PUT", "/api/v1/templates/repository", `{"url":"https://example.test/a.json","allow_private":false}`, strings.Repeat("a", 64))
	if w.Code != 200 {
		t.Fatalf("repositorio: %d %s", w.Code, w.Body.String())
	}
	w = call("GET", "/api/v1/templates/repository/preview", "", strings.Repeat("a", 64))
	if w.Code != 200 {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
}
