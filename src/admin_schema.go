package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/gentedelcondado/headscale-gateway/internal/admin"
)

// Las opciones se comparten entre validación Go y la API del panel. Solo se
// publican proveedores con implementación real (no entradas ficticias).
var supportedDNSProviders = []string{"none", "technitium", "cloudflare"}
var supportedACMEDNSProviders = []string{"none", "technitium", "cloudflare"}
var supportedCertificateModes = []string{"auto", "force-local", "force-acme"}

func supportedOption(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func labeledOptions(values []string, names map[string]string) []admin.ConfigOption {
	options := make([]admin.ConfigOption, 0, len(values))
	for _, value := range values {
		label := names[value]
		if label == "" {
			label = value
		}
		options = append(options, admin.ConfigOption{Value: value, Label: label})
	}
	return options
}

// configFields define el formulario en un único punto. La lista de plantillas
// se obtiene de SQLite, no de constantes JS.
func configFields(templateNames []string) []admin.ConfigField {
	certModes := labeledOptions(supportedCertificateModes, map[string]string{
		"auto": "Automático (ACME o CA local)", "force-local": "Solo CA local", "force-acme": "Solo ACME",
	})
	providers := labeledOptions(supportedDNSProviders, map[string]string{
		"none": "Desactivado", "technitium": "Technitium DNS", "cloudflare": "Cloudflare",
	})
	acmeProviders := labeledOptions(supportedACMEDNSProviders, map[string]string{
		"none": "Desactivado", "technitium": "Technitium DNS-01", "cloudflare": "Cloudflare DNS-01",
	})
	templates := make([]admin.ConfigOption, 0, len(templateNames)+1)
	templates = append(templates, admin.ConfigOption{Value: "", Label: "Sin plantilla predeterminada"})
	for _, name := range templateNames {
		templates = append(templates, admin.ConfigOption{Value: name, Label: name})
	}
	boolOptions := []admin.ConfigOption{{Value: "true", Label: "Activado"}, {Value: "false", Label: "Desactivado"}}
	return []admin.ConfigField{
		{Key: "pki.discovery_port", Label: "Puerto UDP de descubrimiento LAN", Group: "Comunicaciones LAN", Kind: "text", Description: "Por defecto 47991. Todos los gateways que se busquen por LAN deben usar el mismo puerto. Reinicio requerido."},
		{Key: "pki.http_enabled", Label: "Publicar CA por HTTP en LAN", Group: "Comunicaciones LAN", Kind: "select", Options: boolOptions, Description: "Solo el certificado público, nunca administración ni claves privadas. Requiere activar también Publicar CA en CA y confianza. Reinicio requerido."},
		{Key: "pki.http_listen_addr", Label: "Dirección y puerto HTTP de la CA", Group: "Comunicaciones LAN", Kind: "text", Description: "Por ejemplo 0.0.0.0:9080. Elige otro puerto si está ocupado. No cambia el puerto HTTPS ni el de administración."},
		{Key: "pki.allow_http_download", Label: "Permitir descarga de CA por HTTP LAN", Group: "Comunicaciones LAN", Kind: "select", Options: boolOptions, Description: "Último recurso después de HTTPS. La CA queda pendiente: verifica la huella por un canal independiente antes de aprobarla."},
		{Key: "server.listen_addr", Label: "Escucha HTTPS LAN", Group: "Comunicaciones LAN", Kind: "text", Description: "Dirección y puerto TCP del gateway; por ejemplo 0.0.0.0:8443. Admite puertos personalizados; requiere reiniciar. Tailscale conserva HTTPS 443."},
		{Key: "domain.base_domain", Label: "Dominio base", Group: "Servidor", Kind: "text", Required: true},
		{Key: "headscale.url", Label: "URL de Headscale (control de la VPN)", Group: "Headscale", Kind: "url", Required: true},
		{Key: "headscale.panel_enabled", Label: "Panel de red Headscale/Tailscale", Group: "Headscale", Kind: "select", Options: boolOptions, Description: "Permite consultar nodos, DERP y ejecutar diagnósticos. Se aplica al reiniciar."},
		{Key: "headscale.api_key", Label: "Clave API de Headscale", Group: "Headscale", Kind: "secret", Description: "Generar en el VPS con headscale apikeys create. Nunca se envía al navegador; dejar vacío la conserva.", VisibleWhen: map[string][]string{"headscale.panel_enabled": {"true"}}},
		{Key: "dns.provider", Label: "Proveedor de registros DNS", Group: "DNS", Kind: "select", Options: providers, Description: "Solo proveedores integrados en esta versión."},
		{Key: "dns.zone", Label: "Zona DNS", Group: "DNS", Kind: "text", Description: "Zona de publicación de registros A; para Cloudflare puede ser una subzona del dominio del Zone ID.", VisibleWhen: map[string][]string{"dns.provider": {"technitium", "cloudflare"}, "acme.dns_provider": {"technitium"}}},
		{Key: "dns.technitium.url", Label: "URL de Technitium", Group: "DNS", Kind: "url", VisibleWhen: map[string][]string{"dns.provider": {"technitium"}, "acme.dns_provider": {"technitium"}}},
		{Key: "dns.technitium.token", Label: "Token de Technitium", Group: "DNS", Kind: "secret", Description: "No se muestra el token actual. Deja el campo vacío para conservarlo.", VisibleWhen: map[string][]string{"dns.provider": {"technitium"}, "acme.dns_provider": {"technitium"}}},
		{Key: "dns.cloudflare.zone_id", Label: "Zone ID de Cloudflare (registros A)", Group: "DNS", Kind: "text", Description: "Identificador de zona de Cloudflare, no el nombre de dominio.", VisibleWhen: map[string][]string{"dns.provider": {"cloudflare"}}},
		{Key: "dns.cloudflare.token", Label: "Token Cloudflare (registros A)", Group: "DNS", Kind: "secret", Description: "Permisos DNS Read y DNS Edit para esa zona. Dejar vacío conserva el actual.", VisibleWhen: map[string][]string{"dns.provider": {"cloudflare"}}},
		{Key: "acme.dns_provider", Label: "Proveedor DNS-01 (ACME)", Group: "Certificados", Kind: "select", Options: acmeProviders, Description: "Solo implementaciones de challenge DNS-01 disponibles."},
		{Key: "acme.cloudflare.zone_id", Label: "Zone ID de Cloudflare (DNS-01)", Group: "Certificados", Kind: "text", Description: "Zona pública autoritativa para los desafíos ACME.", VisibleWhen: map[string][]string{"acme.dns_provider": {"cloudflare"}}},
		{Key: "acme.cloudflare.token", Label: "Token Cloudflare (DNS-01)", Group: "Certificados", Kind: "secret", Description: "Token independiente, con permisos DNS Edit en esa zona.", VisibleWhen: map[string][]string{"acme.dns_provider": {"cloudflare"}}},
		{Key: "acme.email", Label: "Correo ACME", Group: "Certificados", Kind: "email", Description: "Obligatorio salvo en el modo Solo CA local."},
		{Key: "certificates.mode", Label: "Modo de certificados", Group: "Certificados", Kind: "select", Options: certModes, Required: true},
		{Key: "certificates.renew_before", Label: "Renovar antes de", Group: "Certificados", Kind: "duration", Description: "Duración de Go; por ejemplo 720h."},
		{Key: "certificates.local_ca_validity", Label: "Validez de la CA local", Group: "Certificados", Kind: "duration", Description: "Por ejemplo 87600h."},
		{Key: "certificates.local_certificate_validity", Label: "Validez de certificado local", Group: "Certificados", Kind: "duration", Description: "Por ejemplo 8760h."},
		{Key: "proxy.default_egress", Label: "Ruta saliente predeterminada", Group: "Servicios", Kind: "select", Options: []admin.ConfigOption{{Value: "system", Label: "Sistema operativo / tailscaled"}, {Value: "tailnet", Label: "Identidad propia del Gateway / tsnet"}}, Description: "Se aplica a servicios que no indiquen modo propio o en su plantilla. Cambiarlo requiere reiniciar; si tsnet no está disponible, los servicios que lo utilizan devolverán HTTP 502."},
		{Key: "services.default_template", Label: "Plantilla predeterminada", Group: "Servicios", Kind: "select", Options: templates, Description: "Plantillas disponibles en SQLite."},
		{Key: "admin.enabled", Label: "API y panel habilitados", Group: "Administración", Kind: "select", Options: boolOptions, Description: "Desactivar impide acceder a la administración después del reinicio."},
		{Key: "admin.listen_addr", Label: "Escucha API/panel", Group: "Administración", Kind: "text", Description: "Acceso HTTP local de recuperación. Mantener 127.0.0.1:9091; nunca abrir este puerto a Internet."},
		{Key: "admin.domain_mode", Label: "Acceso HTTPS por dominio", Group: "Administración", Kind: "select", Options: []admin.ConfigOption{{Value: "off", Label: "Desactivado (predeterminado)"}, {Value: "tailnet", Label: "Solo Headscale / red privada"}, {Value: "all", Label: "Headscale y LAN (avanzado)"}}, Description: "Sirve admin.<nodo>.<dominio-base> por HTTPS en el listener existente; no expone 9091. Headscale usa :443; LAN utiliza el puerto de escucha HTTPS. Requiere reiniciar."},
		{Key: "admin.subdomain", Label: "Nombre del subdominio administrativo", Group: "Administración", Kind: "text", Description: "Etiqueta DNS, por defecto admin. Se crea el registro A mediante el proveedor DNS configurado.", VisibleWhen: map[string][]string{"admin.domain_mode": {"tailnet", "all"}}},
	}
}

func (a *adminController) ConfigSchema(_ context.Context) ([]admin.ConfigField, error) {
	a.runtime.transitionMu.Lock()
	defer a.runtime.transitionMu.Unlock()
	return a.configSchemaLocked()
}

// configSchemaLocked lee el catálogo actual de plantillas de SQLite.
func (a *adminController) configSchemaLocked() ([]admin.ConfigField, error) {
	db, err := openCatalog(a.configPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.ListTemplates()
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.Name)
	}
	sort.Strings(names)
	return configFields(names), nil
}
func (a *adminController) validateSettingChoicesLocked(changes map[string]string) error {
	fields, err := a.configSchemaLocked()
	if err != nil {
		return err
	}
	for _, field := range fields {
		if field.Kind != "select" {
			continue
		}
		value, changed := changes[field.Key]
		if !changed {
			continue
		}
		matched := false
		for _, opt := range field.Options {
			if opt.Value == value {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: opción no admitida %q", field.Key, value)
		}
	}
	return nil
}
