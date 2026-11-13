package main

// Modelos de configuración en tiempo de ejecución y de opciones de proxy.
import (
	"github.com/gentedelcondado/headscale-gateway/internal/portal"
	"time"
)

type Config struct {
	Portal          portal.Config
	BaseDomain      string
	DNSProvider     string
	DNSZone         string
	ACMEDNSProvider string

	// URL y token compartidos entre la gestión DNS y ACME DNS-01.
	// El token nunca debe aparecer en los logs.
	ProviderDNSURL   string
	ProviderDNSToken string

	// Cloudflare: credenciales independientes para DNS A y ACME DNS-01.
	CloudflareDNSZoneID  string
	CloudflareDNSToken   string
	CloudflareACMEZoneID string
	CloudflareACMEToken  string

	HeadscaleURL        string
	HeadscaleAPIKey     string
	NetworkPanelEnabled bool
	ACMEEmail           string
	DataDir             string
	DatabasePath        string // Ruta absoluta del catálogo: estado ACME y configuración

	// Vista inmutable de los servicios SQLite para el motor de proxy/DNS.
	Services          map[string]string
	ServiceProxy      map[string]ProxyOptions
	DefaultTemplate   string
	DefaultEgressMode string // system o tailnet; fallback global para servicios sin override
	DiscoveryPort     int
	CAHTTPEnabled     bool
	CAHTTPListenAddr  string
	CAHTTPDownload    bool
	LocalListenAddr   string
	AdminEnabled      bool
	AdminListenAddr   string
	AdminDomainMode   string // off, tailnet, all (HTTPS only)
	AdminSubdomain    string // single DNS label, default admin

	LocalCertificateIPs      []string // Snapshot of gateway LAN, loopback and tsnet IPs; never ports.
	CertificateMode          string
	CertificateRenewBefore   time.Duration
	LocalCAValidity          time.Duration
	LocalCertificateValidity time.Duration
}

type ProxyOptions struct {
	EgressMode            string // system (SO/tailscaled) o tailnet (tsnet del Gateway)
	PreserveHost          bool
	Timeout               time.Duration
	ResponseHeaderTimeout time.Duration
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnections        int
	InsecureSkipVerify    bool
	TLSServerName         string
	RequestHeaders        map[string]string
	ResponseHeaders       map[string]string
	RedirectTo            string
	RedirectStatus        int
}

type storedProxyOptions struct {
	EgressMode            string `json:"egress_mode,omitempty"`
	PreserveHost          *bool  `json:"preserve_host"`
	Timeout               string `json:"timeout"`
	ResponseHeaderTimeout string `json:"response_header_timeout"`
	DialTimeout           string `json:"dial_timeout"`
	TLSHandshakeTimeout   string `json:"tls_handshake_timeout"`
	IdleConnTimeout       string `json:"idle_conn_timeout"`
	MaxIdleConns          *int   `json:"max_idle_conns"`
	MaxIdleConnsPerHost   *int   `json:"max_idle_conns_per_host"`
	MaxConnections        *int   `json:"max_connections"`
	TLS                   struct {
		InsecureSkipVerify *bool   `json:"insecure_skip_verify"`
		ServerName         *string `json:"server_name"`
	} `json:"tls"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	RedirectTo      string            `json:"redirect_to"`
	RedirectStatus  int               `json:"redirect_status"`
}
