package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestTechnitiumErrorsKeepCredentialPrivate(t *testing.T) {
	const secret = "test/+sensitive=value"
	for _, status := range []int{0, 403, 200} {
		dns := newTechnitiumDNS(Config{ProviderDNSURL: "https://dns.invalid", ProviderDNSToken: secret})
		dns.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Query().Get("token") != secret {
				t.Fatal("falta el token del proveedor")
			}
			if status == 0 {
				return nil, errors.New("connection refused")
			}
			body := `{"status":"error","errorMessage":"` + secret + `"}`
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		_, err := dns.GetRecords(context.Background(), "node.test", "test", false)
		if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), url.QueryEscape(secret)) {
			t.Fatalf("error inseguro o ausente en el caso HTTP %d", status)
		}
	}
}

// fakeDNS deja comprobar decisiones del Gateway sin Technitium o Internet.
type fakeDNS struct {
	records                 []DNSRecord
	added, updated, deleted int
}

func (f *fakeDNS) GetRecords(context.Context, string, string, bool) ([]DNSRecord, error) {
	return f.records, nil
}
func (f *fakeDNS) AddARecord(context.Context, string, string, string, int, string) error {
	f.added++
	return nil
}
func (f *fakeDNS) UpdateARecord(context.Context, string, string, string, string, int, string) error {
	f.updated++
	return nil
}
func (f *fakeDNS) DeleteARecord(context.Context, string, string, string) error {
	f.deleted++
	return nil
}

func TestDNSNoModificaRegistrosExternos(t *testing.T) {
	f := &fakeDNS{records: []DNSRecord{{Name: "app.node.example", Type: "A", IP: "10.0.0.4", Comments: "admin-manual"}}}
	err := ensureARecord(context.Background(), f, "example", "app.node.example", "100.64.0.5", managedDNSCommentForNode("node.example"))
	if err == nil || f.added+f.updated+f.deleted != 0 {
		t.Fatalf("se modificó DNS externo: %v %+v", err, f)
	}
}
func TestDNSGestionadoSeActualizaSoloUnaVez(t *testing.T) {
	const domain = "node.example"
	managed := managedDNSCommentForNode(domain)
	f := &fakeDNS{records: []DNSRecord{{Name: "app." + domain, Type: "A", IP: "100.64.0.1", Comments: managed}}}
	if err := ensureARecord(context.Background(), f, "example", "app."+domain, "100.64.0.5", managed); err != nil {
		t.Fatal(err)
	}
	if f.updated != 1 || f.added != 0 || f.deleted != 0 {
		t.Fatalf("actualización inesperada: %+v", f)
	}
	f.records = append(f.records, DNSRecord{Name: "app." + domain, Type: "A", IP: "100.64.0.2", Comments: managed})
	if err := ensureARecord(context.Background(), f, "example", "app."+domain, "100.64.0.5", managed); err == nil {
		t.Fatal("duplicados gestionados aceptados")
	}
}
func TestDNSNamespaceEstrictamenteLimitado(t *testing.T) {
	for _, tc := range []struct {
		host, node string
		yes        bool
	}{
		{"node.example", "node.example", true}, {"app.node.example", "node.example", true},
		{"APP.NODE.EXAMPLE.", "node.example", true}, {"othernode.example", "node.example", false},
		{"node.example.evil.org", "node.example", false}, {"", "", false}, {"foo.example", "", false},
	} {
		if got := isGatewayDomain(tc.host, tc.node); got != tc.yes {
			t.Errorf("isGatewayDomain(%q,%q)=%v", tc.host, tc.node, got)
		}
	}
}
