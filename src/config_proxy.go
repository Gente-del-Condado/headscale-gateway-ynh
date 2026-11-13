package main

// Analizador de opciones de proxy, anulaciones y validación de encabezados HTTP.
import (
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

func parseProxyOptions(raw storedProxyOptions) (ProxyOptions, error) {
	if raw.EgressMode != "" && raw.EgressMode != "system" && raw.EgressMode != "tailnet" {
		return ProxyOptions{}, fmt.Errorf("proxy.egress_mode inválido: %q (system o tailnet)", raw.EgressMode)
	}
	o := ProxyOptions{EgressMode: raw.EgressMode, PreserveHost: true, ResponseHeaderTimeout: 30 * time.Second, DialTimeout: 10 * time.Second, TLSHandshakeTimeout: 10 * time.Second, IdleConnTimeout: 90 * time.Second, MaxIdleConns: 100, MaxIdleConnsPerHost: 20, RequestHeaders: raw.RequestHeaders, ResponseHeaders: raw.ResponseHeaders, RedirectTo: raw.RedirectTo, RedirectStatus: raw.RedirectStatus}
	if raw.TLS.InsecureSkipVerify != nil {
		o.InsecureSkipVerify = *raw.TLS.InsecureSkipVerify
	}
	if raw.TLS.ServerName != nil {
		o.TLSServerName = *raw.TLS.ServerName
	}
	if raw.MaxConnections != nil {
		o.MaxConnections = *raw.MaxConnections
	}
	if raw.PreserveHost != nil {
		o.PreserveHost = *raw.PreserveHost
	}
	for _, item := range []struct {
		name, value string
		dest        *time.Duration
	}{{"timeout", raw.Timeout, &o.Timeout}, {"response_header_timeout", raw.ResponseHeaderTimeout, &o.ResponseHeaderTimeout}, {"dial_timeout", raw.DialTimeout, &o.DialTimeout}, {"tls_handshake_timeout", raw.TLSHandshakeTimeout, &o.TLSHandshakeTimeout}, {"idle_conn_timeout", raw.IdleConnTimeout, &o.IdleConnTimeout}} {
		if item.value == "" {
			continue
		}
		d, err := time.ParseDuration(item.value)
		if err != nil || d < 0 {
			return o, fmt.Errorf("proxy.%s inválido: %q", item.name, item.value)
		}
		*item.dest = d
	}
	if raw.MaxIdleConns != nil {
		o.MaxIdleConns = *raw.MaxIdleConns
	}
	if raw.MaxIdleConnsPerHost != nil {
		o.MaxIdleConnsPerHost = *raw.MaxIdleConnsPerHost
	}
	if o.MaxConnections < 0 || o.MaxIdleConns < 0 || o.MaxIdleConnsPerHost < 0 {
		return o, fmt.Errorf("límites de conexiones negativos")
	}
	if o.RedirectTo != "" {
		u, err := url.Parse(o.RedirectTo)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return o, fmt.Errorf("redirect_to requiere URL absoluta http(s)")
		}
		if o.RedirectStatus == 0 {
			o.RedirectStatus = 308
		}
	}
	if o.RedirectStatus != 0 && o.RedirectStatus != 301 && o.RedirectStatus != 302 && o.RedirectStatus != 307 && o.RedirectStatus != 308 {
		return o, fmt.Errorf("redirect_status debe ser 301, 302, 307 o 308")
	}
	for _, headers := range []map[string]string{o.RequestHeaders, o.ResponseHeaders} {
		for key, value := range headers {
			if !validProxyHeader(key, value) {
				return o, fmt.Errorf("cabecera HTTP no válida: %q", key)
			}
		}
	}
	return o, nil
}

// mergeProxy respeta valores explícitos false, 0 y "0s".
func mergeProxy(base, override storedProxyOptions) storedProxyOptions {
	originalRequest := base.RequestHeaders
	originalResponse := base.ResponseHeaders
	a := reflect.ValueOf(&base).Elem()
	b := reflect.ValueOf(override)
	for i := 0; i < a.NumField(); i++ {
		field := b.Field(i)
		switch field.Kind() {
		case reflect.Ptr, reflect.Map:
			if !field.IsNil() {
				a.Field(i).Set(field)
			}
		case reflect.String:
			if field.String() != "" {
				a.Field(i).Set(field)
			}
		case reflect.Int:
			if field.Int() != 0 {
				a.Field(i).Set(field)
			}
		case reflect.Struct:
			for j := 0; j < field.NumField(); j++ {
				if !field.Field(j).IsNil() {
					a.Field(i).Field(j).Set(field.Field(j))
				}
			}
		}
	}
	// Las cabeceras se fusionan por nombre, con preferencia del servicio.
	for _, pair := range []struct {
		dst      *map[string]string
		original map[string]string
		newer    map[string]string
	}{
		{&base.RequestHeaders, originalRequest, override.RequestHeaders},
		{&base.ResponseHeaders, originalResponse, override.ResponseHeaders},
	} {
		if pair.newer == nil {
			continue
		}
		merged := make(map[string]string, len(pair.original)+len(pair.newer))
		for k, v := range pair.original {
			merged[k] = v
		}
		for k, v := range pair.newer {
			merged[k] = v
		}
		*pair.dst = merged
	}
	return base
}

// initialSettings es la única fuente de valores predeterminados. Un dominio base y
// una URL de Headscale vacíos mantienen deliberadamente el panel de administración en modo de configuración.
// validProxyHeader rechaza nombres, valores y cabeceras de transporte peligrosas.
func validProxyHeader(name, value string) bool {
	if !httpgutsValidToken(name) || strings.ContainsAny(value, "\r\n") {
		return false
	}
	switch strings.ToLower(name) {
	case "host", "connection", "upgrade", "transfer-encoding", "content-length", "te", "trailer", "proxy-authorization", "proxy-authenticate", "keep-alive", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto", "x-gateway-service":
		return false
	}
	return true
}
func httpgutsValidToken(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	return true
}
