package main

import (
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
)

func TestMainPortalShowsAdminOnlyWhenPublished(t *testing.T) {
	node := "computer.intra.example.test"
	for _, test := range []struct {
		admin string
		want  bool
	}{
		{"", false}, {"admin." + node, true},
	} {
		handler := newGatewayHandler(node, map[string]*httputil.ReverseProxy{}, test.admin)
		req := httptest.NewRequest("GET", "https://"+node+"/", nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != 200 {
			t.Fatalf("portal returned %d", res.Code)
		}
		link := "https://admin." + node + "/admin/"
		if got := strings.Contains(res.Body.String(), link); got != test.want {
			t.Fatalf("admin link visible=%t, want=%t", got, test.want)
		}
	}
}
