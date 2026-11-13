package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyUpgradeWithLimitsIsBidirectional(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n"); err != nil {
			t.Error(err)
			return
		}
		if err := rw.Flush(); err != nil {
			t.Error(err)
			return
		}
		if _, err := io.CopyN(conn, rw, 4); err != nil {
			t.Error(err)
		}
	}))
	defer backend.Close()
	for name, options := range map[string]ProxyOptions{
		"timeout": {Timeout: 10 * time.Second},
		"limit":   {MaxConnections: 1},
		"both":    {MaxConnections: 1, Timeout: 10 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			proxies := buildProxyMapAdvanced(map[string]string{"app": backend.URL}, map[string]ProxyOptions{"app": options}, "node.test")
			frontend := httptest.NewServer(proxies["app.node.test"])
			defer frontend.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, frontend.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			res, err := frontend.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusSwitchingProtocols {
				t.Fatal(res.Status)
			}
			stream, ok := res.Body.(io.ReadWriteCloser)
			if !ok {
				t.Fatal("el túnel no permite escritura")
			}
			if _, err := io.WriteString(stream, "ping"); err != nil {
				t.Fatal(err)
			}
			var echo [4]byte
			if _, err := io.ReadFull(stream, echo[:]); err != nil || string(echo[:]) != "ping" {
				t.Fatal("no se completó el intercambio a través del proxy", err)
			}
		})
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestProxyCloseLiberaUnaSolaVez(t *testing.T) {
	var count atomic.Int32
	body := &proxyCloseBody{ReadCloser: io.NopCloser(strings.NewReader("ok")), done: func() { count.Add(1) }}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if count.Load() != 1 {
		t.Fatalf("liberaciones=%d", count.Load())
	}
}

func TestProxyLimitTransportLiberacionIdempotente(t *testing.T) {
	limiter := &proxyLimitTransport{next: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}), slots: make(chan struct{}, 1)}
	req, _ := http.NewRequest("GET", "http://backend.invalid/", nil)
	resp, err := limiter.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = resp.Body.Close(); _ = resp.Body.Close() }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("bloqueo al cerrar dos veces")
	}
	if len(limiter.slots) != 0 {
		t.Fatal("slot no liberado")
	}
}

func TestProxyLimitTransportRespetaCancelacion(t *testing.T) {
	limiter := &proxyLimitTransport{slots: make(chan struct{}, 1)}
	limiter.slots <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://backend.invalid/", nil)
	_, err := limiter.RoundTrip(req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelación ignorada: %v", err)
	}
}
