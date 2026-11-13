package main

// Presets versionados del catálogo. Se instalan sólo durante el bootstrap de
// SQLite y nunca se actualizan silenciosamente si el administrador los edita.
// Las URLs de ejemplo son locales: todos los servicios se crean DESACTIVADOS.
import (
	"encoding/json"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
)

var builtinTemplateJSON = map[string]string{
	"default":        `{"preserve_host":true,"dial_timeout":"10s","tls_handshake_timeout":"10s","response_header_timeout":"30s","idle_conn_timeout":"90s","max_idle_conns":100,"max_idle_conns_per_host":20}`,
	"streaming":      `{"preserve_host":true,"timeout":"0s","response_header_timeout":"0s","dial_timeout":"15s","idle_conn_timeout":"120s","max_idle_conns_per_host":32}`,
	"private-tls":    `{"preserve_host":true,"tls":{"insecure_skip_verify":false},"tls_handshake_timeout":"10s"}`,
	"websocket":      `{"preserve_host":true,"timeout":"0s","response_header_timeout":"0s","idle_conn_timeout":"120s"}`,
	"home-assistant": `{"preserve_host":true,"timeout":"0s","response_header_timeout":"0s","dial_timeout":"10s","idle_conn_timeout":"120s","max_idle_conns_per_host":32,"request_headers":{"X-Gateway-App":"home-assistant"}}`,
	"immich":         `{"preserve_host":true,"timeout":"0s","response_header_timeout":"600s","dial_timeout":"15s","idle_conn_timeout":"120s","max_idle_conns_per_host":32,"request_headers":{"X-Gateway-App":"immich"}}`,
	"cockpit":        `{"preserve_host":true,"timeout":"0s","response_header_timeout":"0s","dial_timeout":"10s","tls":{"insecure_skip_verify":false},"request_headers":{"X-Gateway-App":"cockpit"}}`,
	"calibre":        `{"preserve_host":true,"timeout":"0s","response_header_timeout":"90s","dial_timeout":"10s","request_headers":{"X-Gateway-App":"calibre"}}`,
	"vanguard":       `{"preserve_host":true,"timeout":"0s","response_header_timeout":"60s","dial_timeout":"10s","request_headers":{"X-Gateway-App":"vanguard"}}`,
}

var builtinTemplateMeta = map[string]struct{ category, description, backend string }{
	"default":        {"General", "Proxy HTTP estándar. Preserva Host, valida TLS y establece límites de conexión seguros.", "http://127.0.0.1:8080"},
	"streaming":      {"Multimedia", "Sin timeout global de backend, con conexiones persistentes para streaming.", "http://127.0.0.1:8096"},
	"private-tls":    {"Seguridad", "Backend HTTPS validado; configurar tls.server_name si el certificado no coincide con la IP.", "https://servidor-interno:443"},
	"websocket":      {"Tiempo real", "WebSocket automático mediante ReverseProxy y espera sin límite de cabeceras.", "http://127.0.0.1:3000"},
	"home-assistant": {"Domótica", "WebSocket /api/websocket y cabeceras X-Forwarded. Configurar trusted_proxies en Home Assistant.", "http://127.0.0.1:8123"},
	"immich":         {"Fotografía", "Subidas y descargas grandes, tiempos de backend ampliados y publicación en raíz del subdominio.", "http://127.0.0.1:2283"},
	"cockpit":        {"Administración", "Conserva Host y TLS verificado al backend. Configurar Origins y nombre TLS del certificado.", "https://127.0.0.1:9090"},
	"calibre":        {"Libros", "Perfil Calibre Content Server; Calibre-Web suele utilizar otro puerto (habitualmente 8083).", "http://127.0.0.1:8080"},
	"vanguard":       {"Personalizado", "Perfil HTTP genérico: definir puerto, servidor y requisitos de la aplicación Vanguard específica.", "http://127.0.0.1:8080"},
}

var builtinServices = []state.ServiceRecord{
	{Name: "home-assistant", Description: "Control domótico; configurar trusted_proxies y URL real", Enabled: false, Backend: "http://127.0.0.1:8123", Template: "home-assistant"},
	{Name: "immich", Description: "Fotos y vídeos; puerto habitual 2283", Enabled: false, Backend: "http://127.0.0.1:2283", Template: "immich"},
	// Cockpit suele escuchar HTTPS en :9090. Se mantiene verificación TLS;
	// debe configurarse un nombre/certificado válido para el backend.
	{Name: "cockpit", Description: "Administración de sistemas; backend HTTPS", Enabled: false, Backend: "https://127.0.0.1:9090", Template: "cockpit"},
	{Name: "calibre", Description: "Biblioteca de libros; Calibre Content Server", Enabled: false, Backend: "http://127.0.0.1:8080", Template: "calibre"},
	// Vanguard puede referirse a varias aplicaciones: URL ilustrativa.
	{Name: "vanguard", Description: "Personalizar aplicación y URL del backend", Enabled: false, Backend: "http://127.0.0.1:8080", Template: "vanguard"},
}

func seedCatalogPresets(db *state.Store) error {
	existingTemplates, err := db.ListTemplates()
	if err != nil {
		return err
	}
	knownTemplates := make(map[string]bool, len(existingTemplates))
	for _, t := range existingTemplates {
		knownTemplates[t.Name] = true
	}
	for name, source := range builtinTemplateJSON {
		if knownTemplates[name] {
			continue
		}
		if !json.Valid([]byte(source)) {
			return fmt.Errorf("preset %s: JSON inválido", name)
		}
		var options storedProxyOptions
		if err := json.Unmarshal([]byte(source), &options); err != nil {
			return err
		}
		if _, err := parseProxyOptions(options); err != nil {
			return fmt.Errorf("preset %s: %w", name, err)
		}
		meta := builtinTemplateMeta[name]
		if err := db.SaveTemplate(state.TemplateRecord{Name: name, Proxy: json.RawMessage(source), Category: meta.category, Description: meta.description, BackendHint: meta.backend}); err != nil {
			return err
		}
	}
	existingServices, err := db.ListServices()
	if err != nil {
		return err
	}
	knownServices := make(map[string]bool, len(existingServices))
	for _, service := range existingServices {
		knownServices[service.Name] = true
	}
	for _, svc := range builtinServices {
		if knownServices[svc.Name] {
			continue
		}
		if err := db.SaveService(svc); err != nil {
			return err
		}
	}
	return nil
}
