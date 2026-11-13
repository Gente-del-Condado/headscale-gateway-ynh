package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeHeadscaleManagement struct {
	fakeNetwork
	action HeadscaleAction
}

func (f *fakeHeadscaleManagement) HeadscaleUsers(context.Context) (any, error) {
	return map[string]any{"users": []any{}}, nil
}
func (f *fakeHeadscaleManagement) HeadscalePreAuthKeys(context.Context) (any, error) {
	return map[string]any{"preAuthKeys": []any{}}, nil
}
func (f *fakeHeadscaleManagement) HeadscaleManage(_ context.Context, action HeadscaleAction) (any, error) {
	f.action = action
	return map[string]any{"ok": true}, nil
}

func TestHeadscaleManagementRequiresAuthentication(t *testing.T) {
	controller := &fakeHeadscaleManagement{}
	server, err := New(strings.Repeat("M", 64), controller)
	if err != nil {
		t.Fatal(err)
	}
	handler := server.Handler()
	call := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}
	if w := call("GET", "/api/v1/network/admin/users", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected users: %d", w.Code)
	}
	if w := call("GET", "/api/v1/network/admin/keys", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected keys: %d", w.Code)
	}
	if w := call("POST", "/api/v1/network/admin/action", `{"action":"node.delete","id":"1"}`, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("unprotected mutation: %d", w.Code)
	}
	token := strings.Repeat("M", 64)
	if w := call("GET", "/api/v1/network/admin/users", "", token); w.Code != 200 {
		t.Fatalf("user list: %d", w.Code)
	}
	if w := call("GET", "/api/v1/network/admin/keys", "", token); w.Code != 200 {
		t.Fatalf("key list: %d", w.Code)
	}
	if w := call("POST", "/api/v1/network/admin/action", `{"action":"node.rename","id":"42","name":"office"}`, token); w.Code != 200 || controller.action.Action != "node.rename" {
		t.Fatalf("mutation %d: %+v", w.Code, controller.action)
	}
	if w := call("POST", "/api/v1/network/admin/action", `{"action":"node.rename","invalid":123}`, token); w.Code != 400 {
		t.Fatalf("unknown JSON accepted: %d", w.Code)
	}
}
