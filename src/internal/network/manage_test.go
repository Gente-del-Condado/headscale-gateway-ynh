package network

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHeadscaleAdminOperations(t *testing.T) {
	var method, path, auth string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.RequestURI(), r.Header.Get("Authorization")
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	a := HeadscaleAdmin{BaseURL: server.URL, APIKey: "dummy-test-token"}
	if _, err := a.RenameNode(context.Background(), "7", "my-node"); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/v1/node/7/rename/my-node" || auth != "Bearer dummy-test-token" {
		t.Fatalf("incorrect request %s %s %s", method, path, auth)
	}
	if _, err := a.SetApprovedRoutes(context.Background(), "7", []string{"192.168.10.0/24"}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/node/7/approve_routes" || payload["routes"] == nil {
		t.Fatalf("routes: %s %+v", path, payload)
	}
	if _, err := a.SetTags(context.Background(), "7", []string{"tag:server"}); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/node/7/tags" || payload["tags"] == nil {
		t.Fatalf("tags: %s %+v", path, payload)
	}
	if _, err := a.DeleteNode(context.Background(), "7"); err != nil {
		t.Fatal(err)
	}
	if method != "DELETE" || path != "/api/v1/node/7" {
		t.Fatal(method, path)
	}
	if _, err := a.CreatePreAuthKey(context.Background(), "4", false, false, 24, []string{}); err != nil {
		t.Fatal(err)
	}
	if method != "POST" || path != "/api/v1/preauthkey" || payload["user"] != "4" {
		t.Fatalf("key request: %s %+v", path, payload)
	}
}
func TestHeadscaleRejectsUnsafeParameters(t *testing.T) {
	a := HeadscaleAdmin{BaseURL: "http://127.0.0.1:1", APIKey: "test"}
	for _, id := range []string{"../../evil", "1/2", "1?foo", "-1"} {
		if _, err := a.DeleteNode(context.Background(), id); err == nil {
			t.Fatal("accepted", id)
		}
	}
	if _, err := a.SetTags(context.Background(), "12", []string{"bad-tag"}); err == nil {
		t.Fatal("bad tag accepted")
	}
	if _, err := a.SetApprovedRoutes(context.Background(), "12", []string{"not-a-prefix"}); err == nil {
		t.Fatal("bad cidr accepted")
	}
	if _, err := a.CreatePreAuthKey(context.Background(), "1", false, false, 24*91, nil); err == nil {
		t.Fatal("invalid ttl accepted")
	}
	if _, err := a.CreateUser(context.Background(), strings.Repeat("x", 90)); err == nil {
		t.Fatal("overlong user accepted")
	}
}
