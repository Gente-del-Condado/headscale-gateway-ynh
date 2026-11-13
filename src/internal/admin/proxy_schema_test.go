package admin

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxySchemaKeysAndTypes(t *testing.T) {
	fields := ProxySchema()
	if len(fields) != 14 {
		t.Fatalf("se esperan 14 campos, hay %d", len(fields))
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if field.Key == "" || field.Label == "" || field.Group == "" || seen[field.Key] {
			t.Fatalf("metadatos de proxy incompletos o repetidos: %+v", field)
		}
		seen[field.Key] = true
		switch field.Kind {
		case "boolean", "duration", "integer", "text", "url", "select", "choice":
		default:
			t.Fatalf("tipo de control desconocido: %q", field.Kind)
		}
	}
	for _, key := range []string{"egress_mode", "preserve_host", "tls.insecure_skip_verify", "redirect_status", "timeout", "max_connections"} {
		if !seen[key] {
			t.Fatalf("campo de proxy obligatorio ausente %q", key)
		}
	}
	// El selector de salida al backend es textual y no debe cambiarse a "select":
	// el editor JavaScript trata "select" como numérico.
	for _, field := range fields {
		if field.Key == "egress_mode" {
			if field.Kind != "choice" || len(field.Options) != 2 {
				t.Fatalf("egress_mode debe ser choice con dos opciones: %+v", field)
			}
		}
	}
}

func TestProxySchemaAuthAndEmbeddedModel(t *testing.T) {
	s, err := New(strings.Repeat("e", 64), &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path, token string
		want        int
	}{
		{"/api/v1/proxy/schema", "", 401},
		{"/api/v1/proxy/schema", strings.Repeat("e", 64), 200},
		{"/admin/proxy-model.js", "", 200},
	} {
		r := httptest.NewRequest("GET", tt.path, nil)
		if tt.token != "" {
			r.Header.Set("Authorization", "Bearer "+tt.token)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != tt.want {
			t.Fatalf("%s: %d (esperado %d)", tt.path, w.Code, tt.want)
		}
		if tt.path == "/api/v1/proxy/schema" && w.Code == 200 {
			var got []ProxyField
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != 14 {
				t.Fatalf("esquema JSON no válido: %v %q", err, w.Body.String())
			}
		}
	}
}
