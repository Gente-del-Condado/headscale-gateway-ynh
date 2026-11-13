package firewall

import (
	"context"
	"testing"
)

func TestLoopbackNoFirewall(t *testing.T) {
	for _, v := range []string{"127.0.0.1:9091", "[::1]:9091"} {
		r, e := Inspect(context.Background(), v)
		if e != nil || r.Remote || r.Status != "local" {
			t.Fatalf("%s %+v %v", v, r, e)
		}
	}
	if _, e := Inspect(context.Background(), "0.0.0.0:99999"); e == nil {
		t.Fatal("puerto inválido")
	}
}
