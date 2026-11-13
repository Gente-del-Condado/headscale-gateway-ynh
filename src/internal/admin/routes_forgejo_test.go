package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestForgejoRequiresInteractiveAdminAndCSRF(t *testing.T) {
	token := strings.Repeat("b", 64)
	server, err := New(token, &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	server.auth = emptyAuthManager("")
	handler := server.Handler()
	session := httptest.NewRecorder()
	csrf := server.auth.CreateSession(session, httptest.NewRequest("POST", "/api/v1/auth/login", nil), "admin")
	cookie := session.Result().Cookies()[0]
	for _, test := range []struct {
		name                string
		cookie              *http.Cookie
		csrf, authorization string
		want                int
	}{
		{name: "anónimo", want: http.StatusUnauthorized},
		{name: "Bearer de recuperación", authorization: "Bearer " + token, want: http.StatusForbidden},
		{name: "sesión sin CSRF", cookie: cookie, want: http.StatusUnauthorized},
		{name: "sesión con CSRF llega al validador", cookie: cookie, csrf: csrf, want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/api/v1/forgejo/list", strings.NewReader(`{"repository":"http://example.invalid/a/b","token":"ficticio","page":1}`))
			request.Header.Set("Content-Type", "application/json")
			if test.cookie != nil {
				request.AddCookie(test.cookie)
			}
			request.Header.Set("X-CSRF-Token", test.csrf)
			request.Header.Set("Authorization", test.authorization)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("código %d, esperado %d", response.Code, test.want)
			}
		})
	}
}
