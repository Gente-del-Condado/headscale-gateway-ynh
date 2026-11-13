package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHeadscalePolicyCRUD(t *testing.T) {
	saved := "{\"grants\": []}"
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fake-token" {
			t.Errorf("missing bearer")
		}
		if r.URL.Path != "/api/v1/policy" {
			t.Errorf("wrong path %s", r.URL.Path)
		}
		if r.Method == http.MethodPut {
			writes++
			var body struct {
				Policy string `json:"policy"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("request: %v", err)
			}
			saved = body.Policy
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"policy": saved, "updatedAt": "2026-10-23T01:01:01Z"})
	}))
	defer server.Close()
	api := HeadscaleAdmin{BaseURL: server.URL, APIKey: "fake-token"}
	current, err := api.GetPolicy(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next := `{ "grants": [], //deny all
 }`
	result, err := api.SetPolicy(context.Background(), next, current.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Policy != next || writes != 1 {
		t.Fatal("did not update")
	}
	if _, err := api.SetPolicy(context.Background(), `{ "grants": [] }`, current.Revision); err == nil {
		t.Fatal("stale revision accepted")
	}
	if _, err := api.SetPolicy(context.Background(), `{ "grants": [`, result.Revision); err == nil {
		t.Fatal("invalid hujson accepted")
	}
	if _, err := api.SetPolicy(context.Background(), "", result.Revision); err == nil {
		t.Fatal("empty policy accepted")
	}
	if _, err := api.SetPolicy(context.Background(), strings.Repeat(" ", 102401), result.Revision); err == nil {
		t.Fatal("giant policy accepted")
	}
	if writes != 1 {
		t.Fatal("unexpected write")
	}
}

func TestHeadscalePolicyRemoteFailureDiagnostics(t *testing.T) {
	for _, code := range []int{401, 403, 404, 500, 503} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"error":"sensitive-upstream-body"}`))
			}))
			defer server.Close()
			api := HeadscaleAdmin{BaseURL: server.URL, APIKey: "fake-token"}
			_, err := api.GetPolicy(context.Background())
			if err == nil || !strings.Contains(err.Error(), fmt.Sprint(code)) {
				t.Fatalf("expected upstream status %d in error: %v", code, err)
			}
			if strings.Contains(err.Error(), "sensitive-upstream-body") {
				t.Fatalf("upstream response body leaked: %v", err)
			}
			var upstream *HeadscaleHTTPError
			if !errors.As(err, &upstream) || upstream.StatusCode != code {
				t.Fatalf("expected identifiable status %d: %v", code, err)
			}
		})
	}
}

func TestMissingPolicyInitialization(t *testing.T) {
	var saved string
	var writes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && saved == "" {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"message":"loading ACL from database: acl policy not found","secret":"never-disclose"}`))
			return
		}
		if r.Method == "PUT" {
			writes++
			var body struct {
				Policy string `json:"policy"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			saved = body.Policy
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"policy": saved})
	}))
	defer server.Close()
	api := HeadscaleAdmin{BaseURL: server.URL, APIKey: "fake-token"}
	initial, err := api.GetPolicy(context.Background())
	if err != nil || !initial.Missing || initial.Revision != "missing" {
		t.Fatalf("missing detection: %+v: %v", initial, err)
	}
	if _, err := api.SetPolicy(context.Background(), `{"grants":[{"src":["*"],"dst":["*"],"ip":["*"]}]}`, "missing"); err == nil {
		t.Fatal("dangerous initial policy bypassed safe wizard")
	}
	if writes != 0 {
		t.Fatal("wrote policy without explicit initialization")
	}
	_, err = api.SetPolicy(context.Background(), "{}", "missing")
	if err != nil || writes != 1 {
		t.Fatalf("initialization failed: %v", err)
	}
	if _, err = api.SetPolicy(context.Background(), `{"grants":[]}`, "missing"); err == nil {
		t.Fatal("stale missing revision accepted")
	}
}
func TestHTTP500UnrelatedNotMarkedMissing(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"error":"database locked token=secret"}`))
	}))
	defer s.Close()
	api := HeadscaleAdmin{BaseURL: s.URL, APIKey: "fake-token"}
	doc, err := api.GetPolicy(context.Background())
	if err == nil || doc.Missing || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unsafe/misclassified: %v %+v", err, doc)
	}
}
