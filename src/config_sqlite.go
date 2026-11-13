package main

// Mapeo tipado desde filas de SQLite persistidas hacia la configuración en tiempo de ejecución.
import (
	"encoding/json"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"github.com/gentedelcondado/headscale-gateway/internal/state"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func initialSettings() map[string]string {
	return map[string]string{
		"admin.enabled": "true", "admin.listen_addr": "127.0.0.1:9091",
		"admin.domain_mode": "off", "admin.subdomain": "admin",
		"pki.discovery_port": "47991", "pki.http_enabled": "false", "pki.http_listen_addr": "0.0.0.0:9080", "pki.allow_http_download": "false",
		"server.listen_addr": "0.0.0.0:8443", "domain.base_domain": "",
		"headscale.url": "", "headscale.panel_enabled": "true", "headscale.api_key": "",
		"dns.provider": "none", "dns.zone": "", "dns.technitium.url": "", "dns.technitium.token": "",
		"dns.cloudflare.zone_id": "", "dns.cloudflare.token": "", "acme.cloudflare.zone_id": "",
		"acme.cloudflare.token": "", "acme.dns_provider": "none", "acme.email": "",
		"certificates.mode": "force-local", "certificates.renew_before": "720h",
		"certificates.local_ca_validity": "87600h", "certificates.local_certificate_validity": "8760h",
		"services.default_template": "", "proxy.default_egress": "system",
	}
}
func openCatalog(dbPath string) (*state.Store, error) {
	db, err := state.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err = db.InitCatalog(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
func ensureSetupDatabase(dbPath string) error {
	db, err := openCatalog(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	settings, err := db.GetSettings()
	if err != nil {
		return err
	}
	if len(settings) == 0 {
		settings = initialSettings()
		if err := startupInitialSettings(settings); err != nil {
			return err
		}
		if err := db.ReplaceSettings(settings); err != nil {
			return err
		}
	}
	// Los presets son insertados una sola vez por instalación, no en cada
	// reinicio. No se sobreescriben servicios editados y una eliminación
	// posterior es permanente.
	var seeded bool
	found, err := db.GetJSON("catalog.presets.v2", &seeded)
	if err != nil {
		return err
	}
	if found && seeded {
		return nil
	}
	if err := seedCatalogPresets(db); err != nil {
		return err
	}
	return db.PutJSON("catalog.presets.v2", true)
}

// loadConfig lee una instantánea de SQLite. Deliberadamente no consulta
// config.yaml, services/*.yaml or templates/*.yaml.
func loadConfig(dbPath string) (Config, error) {
	db, err := openCatalog(dbPath)
	if err != nil {
		return Config{}, err
	}
	defer db.Close()
	settings, err := db.GetSettings()
	if err != nil {
		return Config{}, err
	}
	return configFromSettings(dbPath, settings, db)
}
func configFromSettings(dbPath string, values map[string]string, db *state.Store) (Config, error) {
	get := func(k string) string { return strings.TrimSpace(values[k]) }
	boolSetting := func(k string, fallback bool) (bool, error) {
		v := get(k)
		if v == "" {
			return fallback, nil
		}
		if v == "true" {
			return true, nil
		}
		if v == "false" {
			return false, nil
		}
		return false, fmt.Errorf("%s: se esperaba true o false", k)
	}
	adminEnabled, err := boolSetting("admin.enabled", true)
	if err != nil {
		return Config{}, err
	}
	networkEnabled, err := boolSetting("headscale.panel_enabled", true)
	if err != nil {
		return Config{}, err
	}
	caHTTP, err := boolSetting("pki.http_enabled", false)
	if err != nil {
		return Config{}, err
	}
	caDownload, err := boolSetting("pki.allow_http_download", false)
	if err != nil {
		return Config{}, err
	}
	discoveryPort := 47991
	if value := get("pki.discovery_port"); value != "" {
		var e error
		discoveryPort, e = strconv.Atoi(value)
		if e != nil || discoveryPort < 1 || discoveryPort > 65535 {
			return Config{}, fmt.Errorf("puerto de descubrimiento fuera del intervalo 1..65535")
		}
	}
	cfg := Config{
		DiscoveryPort: discoveryPort,
		CAHTTPEnabled: caHTTP, CAHTTPDownload: caDownload, CAHTTPListenAddr: get("pki.http_listen_addr"),
		BaseDomain: get("domain.base_domain"), DNSProvider: get("dns.provider"), DNSZone: get("dns.zone"),
		ACMEDNSProvider: get("acme.dns_provider"), ProviderDNSURL: get("dns.technitium.url"),
		ProviderDNSToken: get("dns.technitium.token"), CloudflareDNSZoneID: get("dns.cloudflare.zone_id"),
		CloudflareDNSToken: get("dns.cloudflare.token"), CloudflareACMEZoneID: get("acme.cloudflare.zone_id"),
		CloudflareACMEToken: get("acme.cloudflare.token"), HeadscaleURL: get("headscale.url"),
		HeadscaleAPIKey: get("headscale.api_key"), NetworkPanelEnabled: networkEnabled,
		ACMEEmail: get("acme.email"), DataDir: filepath.Dir(dbPath), DatabasePath: dbPath,
		LocalListenAddr: get("server.listen_addr"), AdminEnabled: adminEnabled,
		AdminListenAddr: get("admin.listen_addr"), AdminDomainMode: get("admin.domain_mode"),
		AdminSubdomain: get("admin.subdomain"), CertificateMode: get("certificates.mode"),
		DefaultTemplate: get("services.default_template"), DefaultEgressMode: get("proxy.default_egress"),
		Services: make(map[string]string), ServiceProxy: make(map[string]ProxyOptions),
	}
	if cfg.CAHTTPListenAddr == "" {
		cfg.CAHTTPListenAddr = "0.0.0.0:9080"
	}
	if cfg.DefaultEgressMode == "" {
		cfg.DefaultEgressMode = "system"
	}
	if cfg.DefaultEgressMode != "system" && cfg.DefaultEgressMode != "tailnet" {
		return Config{}, fmt.Errorf("proxy.default_egress debe ser system o tailnet")
	}
	if cfg.DNSProvider == "" {
		cfg.DNSProvider = "none"
	}
	if cfg.ACMEDNSProvider == "" {
		cfg.ACMEDNSProvider = "none"
	}
	if cfg.CertificateMode == "" {
		cfg.CertificateMode = "force-local"
	}
	if cfg.LocalListenAddr == "" {
		cfg.LocalListenAddr = "0.0.0.0:8443"
	}
	if cfg.AdminListenAddr == "" {
		cfg.AdminListenAddr = "127.0.0.1:9091"
	}
	if cfg.AdminDomainMode == "" {
		cfg.AdminDomainMode = "off"
	}
	if cfg.AdminSubdomain == "" {
		cfg.AdminSubdomain = "admin"
	}
	for _, v := range []struct {
		key          string
		dest         *time.Duration
		defaultValue time.Duration
	}{
		{"certificates.renew_before", &cfg.CertificateRenewBefore, 720 * time.Hour},
		{"certificates.local_ca_validity", &cfg.LocalCAValidity, 87600 * time.Hour},
		{"certificates.local_certificate_validity", &cfg.LocalCertificateValidity, 8760 * time.Hour},
	} {
		raw := get(v.key)
		if raw == "" {
			*v.dest = v.defaultValue
			continue
		}
		duration, e := time.ParseDuration(raw)
		if e != nil {
			return Config{}, fmt.Errorf("%s: %w", v.key, e)
		}
		*v.dest = duration
	}
	templates, err := db.ListTemplates()
	if err != nil {
		return Config{}, err
	}
	index := map[string]storedProxyOptions{}
	for _, t := range templates {
		var opts storedProxyOptions
		if len(t.Proxy) > 0 {
			if err = json.Unmarshal(t.Proxy, &opts); err != nil {
				return Config{}, fmt.Errorf("plantilla %s: %w", t.Name, err)
			}
		}
		index[t.Name] = opts
	}
	if cfg.DefaultTemplate != "" {
		if _, ok := index[cfg.DefaultTemplate]; !ok {
			return Config{}, fmt.Errorf("plantilla predeterminada %q no existe", cfg.DefaultTemplate)
		}
	}
	services, err := db.ListServices()
	if err != nil {
		return Config{}, err
	}
	for _, sv := range services {
		if !validAdminServiceName(sv.Name) {
			return Config{}, fmt.Errorf("servicio inválido: %q", sv.Name)
		}
		if !sv.Enabled {
			continue
		}
		selected := sv.Template
		if selected == "" {
			selected = cfg.DefaultTemplate
		}
		var base storedProxyOptions
		if selected != "" {
			var ok bool
			base, ok = index[selected]
			if !ok {
				return Config{}, fmt.Errorf("%s: plantilla %q inexistente", sv.Name, selected)
			}
		}
		var override storedProxyOptions
		if len(sv.Proxy) > 0 {
			if err = json.Unmarshal(sv.Proxy, &override); err != nil {
				return Config{}, fmt.Errorf("%s: proxy JSON inválido: %w", sv.Name, err)
			}
		}
		options, err := parseProxyOptions(mergeProxy(base, override))
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", sv.Name, err)
		}
		if options.EgressMode == "" {
			options.EgressMode = cfg.DefaultEgressMode
		}
		cfg.Services[sv.Name] = strings.TrimSpace(sv.Backend)
		cfg.ServiceProxy[sv.Name] = options
	}
	cfg.Portal = portal.Default()
	if _, err := db.GetJSON(portal.SettingsKey, &cfg.Portal); err != nil {
		return Config{}, err
	}
	if err := cfg.Portal.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
