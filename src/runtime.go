package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"github.com/gentedelcondado/headscale-gateway/internal/licensing"

	"net"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// gatewaySnapshot es inmutable después de publicarse. Las peticiones y los
// handshakes TLS toman la misma generación sin bloquear las actualizaciones.
type gatewaySnapshot struct {
	name, domain string
	handler      http.Handler
	tlsConfig    *tls.Config
	certificates *certificateManager // Mantiene vivo el gestor de certificados.
}

type gatewayRuntime struct {
	license           *licensing.Manager
	lanListening      atomic.Bool
	caHTTPPort        atomic.Int64
	showAdminConsole  func()
	portalSyncMu      sync.Mutex
	portalMu          sync.Mutex
	portalCache       map[string]portalPeerStatus
	cfg               atomic.Pointer[Config]
	transitionMu      sync.Mutex
	dataDir           string
	transport         http.RoundTripper
	active            atomic.Pointer[gatewaySnapshot]
	updateMu          sync.Mutex
	identityConfirmed atomic.Bool
	adminWeb          atomic.Pointer[adminSiteHandler] // handler autenticado compartido por las entradas administrativas
	vpn               *vpnState                        // asignado antes de abrir listeners; solo el supervisor modifica vpn.server
}

func newGatewayRuntime(cfg Config, dataDir, nodeName string) (*gatewayRuntime, error) {
	r := &gatewayRuntime{dataDir: dataDir, transport: newHTTPTransport(), license: licensing.New(dataDir)}
	r.cfg.Store(&cfg)
	// No solicitar ACME con un hostname provisional: Headscale puede
	// renombrarlo (p.ej. equipo -> equipo-1) durante el primer login.
	// LAN usa CA local hasta confirmar el nombre asignado por tsnet.
	provisional := cfg
	provisional.CertificateMode = "force-local"
	first, err := r.prepareConfig(nodeName, provisional)
	if err == nil {
		if cfg.CertificateMode == "force-acme" {
			// force-acme nunca sirve un certificado de CA local, ni siquiera
			// durante el login inicial. El listener TLS espera la identidad
			// confirmada y una emisión ACME válida antes de aceptar clientes.
			first.tlsConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
				GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
					return nil, fmt.Errorf("certificado ACME pendiente de identidad VPN confirmada")
				},
			}
		}
		r.publish(first)
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// prepare construye una generación completa sin modificar la activa.
// Puede tardar durante ACME; las peticiones LAN siguen usando la anterior.
func (r *gatewayRuntime) prepare(nodeName string) (*gatewaySnapshot, error) {
	return r.prepareConfig(nodeName, *r.cfg.Load())
}

func (r *gatewayRuntime) prepareConfig(nodeName string, cfg Config) (*gatewaySnapshot, error) {
	return r.prepareConfigWithRenewal(nodeName, cfg, false)
}

func (r *gatewayRuntime) prepareConfigWithRenewal(nodeName string, cfg Config, refresh bool) (*gatewaySnapshot, error) {
	nodeName = normalizeDNSName(nodeName)
	if nodeName == "" {
		return nil, fmt.Errorf("identidad de nodo vacía")
	}
	// Descubre direcciones locales y la identidad tsnet, ausente de las interfaces del SO.
	if addresses, err := net.InterfaceAddrs(); err == nil {
		for _, address := range addresses {
			if ip, _, err := net.ParseCIDR(address.String()); err == nil {
				cfg.LocalCertificateIPs = append(cfg.LocalCertificateIPs, ip.String())
			}
		}
	}
	if r.vpn != nil {
		r.vpn.mu.Lock()
		server := r.vpn.server
		r.vpn.mu.Unlock()
		if server != nil {
			ip4, ip6 := server.TailscaleIPs()
			if ip4.IsValid() {
				cfg.LocalCertificateIPs = append(cfg.LocalCertificateIPs, ip4.String())
			}
			if ip6.IsValid() {
				cfg.LocalCertificateIPs = append(cfg.LocalCertificateIPs, ip6.String())
			}
		}
	}
	old := r.active.Load()
	domain := nodeName + "." + cfg.BaseDomain
	certDir := filepath.Join(r.dataDir, "certmagic-"+nodeName)
	if err := mkdirPrivate(certDir); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var certs *certificateManager
	var tlsCfg *tls.Config
	if !refresh && old != nil && localCertificateCovers(old.certificates, cfg) && old.name == nodeName && old.domain == domain && old.certificates.mode == cfg.CertificateMode {
		certs = old.certificates
		tlsCfg = old.tlsConfig
	} else {
		manager, err := newCertificateManager(cfg, certDir, domain, "*."+domain, ctx)
		if err != nil {
			return nil, fmt.Errorf("preparando certificados para %s: %w", domain, err)
		}

		certs = manager
		tlsCfg = certs.TLSConfig().Clone()
		tlsCfg.MinVersion = tls.VersionTLS12
		// CertMagic puede devolver NextProtos sin protocolos HTTP.
		// Al reemplazar el tls.Config durante GetConfigForClient,
		// ServeTLS no añade automáticamente h2/http/1.1 a esta copia.
		enableHTTPALPN(tlsCfg)
	}
	// Para cambios exclusivamente de servicios reutilizamos el gestor TLS.
	proxies := buildProxyMapAdvanced(cfg.Services, cfg.ServiceProxy, domain, r.dialTailnet)
	return &gatewaySnapshot{name: nodeName, domain: domain, handler: newGatewayHandler(domain, proxies, func() string {
		if cfg.AdminEnabled && cfg.AdminDomainMode != "off" {
			return cfg.AdminSubdomain + "." + domain
		}
		return ""
	}()), tlsConfig: tlsCfg, certificates: certs}, nil
}

// publish sustituye la generación solo cuando el supervisor ha preparado
// el listener VPN y, si procede, confirmado la sincronización DNS.
// dialTailnet encamina backends usando la identidad tsnet del Gateway.
// No utiliza tailscaled del sistema ni cambia a él si la VPN no está lista.
// La referencia al servidor se consulta en cada conexión para tolerar reinicios.
func (r *gatewayRuntime) dialTailnet(ctx context.Context, network, addr string) (net.Conn, error) {
	if r.vpn == nil {
		return nil, fmt.Errorf("salida tailnet: VPN no configurada")
	}
	r.vpn.mu.Lock()
	server := r.vpn.server
	r.vpn.mu.Unlock()
	if server == nil || !r.identityConfirmed.Load() {
		return nil, fmt.Errorf("salida tailnet: identidad del Gateway no conectada (sin fallback al sistema)")
	}
	return server.Dial(ctx, network, addr)
}

func (r *gatewayRuntime) publish(snap *gatewaySnapshot) {
	r.updateMu.Lock()
	defer r.updateMu.Unlock()
	if r.active.Load() == snap {
		return
	}
	r.active.Store(snap)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-RUNTIME-001", "Generación HTTPS activada: %s", snap.domain)
}

// activate se utiliza para construir la generación inicial LAN.
func (r *gatewayRuntime) activate(nodeName string) error {
	snap, err := r.prepare(nodeName)
	if err != nil {
		return err
	}
	r.publish(snap)
	return nil
}

func (r *gatewayRuntime) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	snap := r.active.Load()
	if snap == nil {
		http.Error(w, "Gateway todavía no está preparado", http.StatusServiceUnavailable)
		return
	}
	if r.servePublishedAdmin(w, req, snap) {
		return
	}
	mainHost := normalizeHost(req.Host) == snap.domain
	if mainHost && req.URL.Path == "/pki/v1/public-ca" && r.servePortal(w, req, snap) {
		return
	}
	if licensing.Enabled && (r.license == nil || !r.license.Status().Allowed) {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, i18n.Translate(i18n.Language(req.Header.Get("Accept-Language")), "Suscripción no activa. Contacta con el administrador del Gateway."), http.StatusPaymentRequired)
		return
	}
	if mainHost && r.servePortal(w, req, snap) {
		return
	}

	snap.handler.ServeHTTP(w, req)
}

// GetConfigForClient se ejecuta en cada nuevo handshake. No modificar nunca
// el tls.Config de una generación ya publicada.
func (r *gatewayRuntime) GetConfigForClient(_ *tls.ClientHelloInfo) (*tls.Config, error) {
	// El inicio en modo configuración puede no tener aún certificado.
	// Devolver un error TLS controlado en lugar de un nil-pointer panic.
	snap := r.active.Load()
	if snap == nil || snap.tlsConfig == nil {
		return nil, fmt.Errorf("certificado HTTPS pendiente de inicialización")
	}
	return snap.tlsConfig, nil
}

// refreshACMELocked sólo sustituye el TLS publicado si ACME ha conseguido
// realmente un certificado válido. Nunca interrumpe el servicio con CA local.
func (r *gatewayRuntime) refreshACME() error {
	if !r.identityConfirmed.Load() {
		return nil
	}
	r.transitionMu.Lock()
	defer r.transitionMu.Unlock()
	snap := r.active.Load()
	if snap == nil || snap.certificates == nil || snap.certificates.localTLSCertificate == nil {
		return nil
	}
	cfg := r.currentConfig()
	if cfg.CertificateMode != "auto" || cfg.ACMEDNSProvider == "none" {
		return nil
	}
	if until := readACMEBackoff(cfg, snap.domain); !until.IsZero() && time.Now().Before(until) {
		return nil
	}
	next, err := r.prepareConfigWithRenewal(snap.name, cfg, true)
	if err != nil {
		return err
	}
	if next.certificates.localTLSCertificate != nil {
		return nil
	}
	r.publish(next)
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-RUNTIME-002", "certificado público instalado para %s sin reiniciar el Gateway", next.domain)
	return nil
}

// Mientras se sirva una CA local por error ACME, revisa si ya puede
// reintentarse. CertMagic se encarga después de sus renovaciones normales.
func refreshCertificatesPeriodically(r *gatewayRuntime) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if err := r.refreshACME(); err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-RUNTIME-003", "actualización ACME aplazada: %v", err)
		}
	}
}

func (r *gatewayRuntime) domain() string {
	if snap := r.active.Load(); snap != nil {
		return snap.domain
	}
	return ""
}

func (r *gatewayRuntime) currentConfig() Config { return *r.cfg.Load() }

func localCertificateCovers(manager *certificateManager, cfg Config) bool {
	if manager == nil {
		return true
	}
	certificate := manager.localTLSCertificate
	if certificate == nil {
		certificate = manager.directTLSCertificate
	}
	if certificate == nil {
		return true
	}
	leaf := certificate.Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return false
		}
	}
	for _, ip := range localCertificateIPs(cfg) {
		if leaf.VerifyHostname(ip.String()) != nil {
			return false
		}
	}
	return leaf.VerifyHostname("localhost") == nil
}
