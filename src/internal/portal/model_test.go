package portal

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModulesDefaultOffAndValidation(t *testing.T) {
	c := Default()
	if c.Publish || c.Federation || c.Discovery {
		t.Fatal("federation enabled by default")
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Accent = "red; background:url(https://evil.test)"
	if c.Validate() == nil {
		t.Fatal("CSS accepted")
	}
	c = Default()
	c.Logo = base64.StdEncoding.EncodeToString([]byte("<svg onload='alert(1)'/>"))
	if c.Validate() == nil {
		t.Fatal("SVG accepted")
	}
}
func TestCatalogConfinement(t *testing.T) {
	good := Catalog{Protocol: "headscale-gateway", Version: 1, Gateway: "Example", Origin: "https://pc.zone.test", Services: []Entry{{ID: "app", Name: "App", URL: "https://app.pc.zone.test"}}}
	if err := good.Validate(good.Origin); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"https://evil.test", "http://app.pc.zone.test", "https://user:pass@app.pc.zone.test", "https://app.pc.zone.test/admin/", "https://app.other.pc.zone.test", "https://app.pc.zone.test:8443", "javascript:alert(1)"} {
		c := good
		c.Services = []Entry{{ID: "app", URL: bad}}
		if c.Validate(good.Origin) == nil {
			t.Fatal("unsafe catalog URL accepted", bad)
		}
	}
}
func TestRenderEscapesAndHasNoAdministrationControls(t *testing.T) {
	c := Default()
	c.Title = "<script>alert(1)</script>"
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "https://pc.zone.test/", nil)
	if err := Render(w, r, c, []Entry{{Name: "<img onerror=alert(1)>", URL: "https://app.pc.zone.test", Gateway: "Peer"}}, ""); err != nil {
		t.Fatal(err)
	}
	body := w.Body.String()
	if strings.Contains(body, "<script>alert(1)") || strings.Contains(body, "<img onerror") || strings.Contains(body, "/api/v1/") {
		t.Fatal(body)
	}
	if !strings.Contains(body, "[hidden]{display:none!important}") {
		t.Fatal("search cannot hide cards")
	}
}
