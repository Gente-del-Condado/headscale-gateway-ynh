package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"crypto/tls"
	"fmt"

	"net"
	"time"

	technitium "github.com/ad-on-is/caddy-dns-technitium"
	"github.com/caddyserver/certmagic"
)

// certificateManager centraliza la gestión de los certificados TLS
// utilizados por el Gateway.
//
// Dependiendo de CertificateMode, el certificado puede proceder de:
//
//   - La CA local persistente.
//   - ACME gestionado mediante CertMagic.
//   - ACME cuando está disponible, con fallback a la CA local.
//
// certConfig se utiliza cuando la gestión ACME está activa.
//
// localTLSCertificate contiene el certificado local cuando el Gateway
// está funcionando con la CA local.
type certificateManager struct {
	mode string

	certConfig *certmagic.Config

	localTLSCertificate  *tls.Certificate
	directTLSCertificate *tls.Certificate // Auto: local CA for IP/localhost, ACME remains preferred for DNS.
}

// newCertificateManager crea y configura el gestor de certificados.
//
// El comportamiento depende de cfg.CertificateMode:
//
//   - force-local:
//     utiliza exclusivamente el certificado generado por la CA local.
//
//   - force-acme:
//     exige ACME mediante el proveedor configurado en
//     acme.dns_provider.
//     Si ACME no funciona, devuelve un error.
//
//   - auto:
//     intenta utilizar ACME cuando acme.dns_provider está configurado.
//     Si ACME no puede utilizarse y no existe un certificado ACME
//     reutilizable, utiliza la CA local como fallback.
//
// nodeDomain representa el dominio del nodo.
//
// wildcardDomain representa el certificado wildcard que cubre
// los servicios publicados bajo el dominio del nodo.
//
// certDir es el almacenamiento persistente utilizado por CertMagic
// para conservar los certificados y su estado.
//
// ctx controla las operaciones de CertMagic/ACME.
func newCertificateManager(
	cfg Config,
	certDir string,
	nodeDomain string,
	wildcardDomain string,
	ctx context.Context,
) (*certificateManager, error) {

	// Creamos el gestor indicando inicialmente el modo configurado.
	manager := &certificateManager{
		mode: cfg.CertificateMode,
	}

	if cfg.CertificateMode == "auto" {
		direct, err := loadLocalCertificate(cfg, nodeDomain)
		if err != nil {
			return nil, fmt.Errorf("preparando TLS local para acceso por IP: %w", err)
		}
		manager.directTLSCertificate = direct
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-001",

		// -------------------------------------------------------------------------
		// FORCE LOCAL
		// -------------------------------------------------------------------------
		"Configurando gestión de certificados...")

	// En este modo no se utiliza ACME ni CertMagic para obtener
	// el certificado servido por el Gateway.
	//
	// Se utiliza exclusivamente la CA local persistente.
	if cfg.CertificateMode == "force-local" {
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-002", "certificates.mode=force-local: usando exclusivamente la CA local")

		// Carga o genera, si es necesario, la CA local y el certificado
		// correspondiente al nodo.
		localTLSCertificate, err := loadLocalCertificate(
			cfg,
			nodeDomain,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"cargando certificado local: %w",
				err,
			)
		}

		manager.localTLSCertificate = localTLSCertificate
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-003", "Certificado TLS local preparado correctamente")

		return manager, nil
	}

	// -------------------------------------------------------------------------
	// CONFIGURACIÓN ACME Y CERTMAGIC
	// -------------------------------------------------------------------------

	// Configuramos el correo utilizado por ACME antes de crear
	// la configuración de CertMagic.
	certmagic.DefaultACME.Email = cfg.ACMEEmail

	// El certificado ACME solicitado contiene dos nombres:
	//
	//   nodo:
	//       equipo-1.intra.example.net
	//
	//   wildcard:
	//       *.equipo-1.intra.example.net
	//
	// De esta forma, el mismo certificado puede cubrir el dominio
	// principal y los servicios publicados bajo él.
	domainNames := []string{
		nodeDomain,
		wildcardDomain,
	}

	// -------------------------------------------------------------------------
	// FORCE ACME
	// -------------------------------------------------------------------------

	// En este modo ACME es obligatorio.
	//
	// A diferencia de "auto", un fallo de ACME no permite continuar
	// utilizando el certificado local.
	if cfg.CertificateMode == "force-acme" {
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-004", "certificates.mode=force-acme: ACME obligatorio")

		// ACME DNS-01 necesita un proveedor configurado explícitamente.
		//
		// La selección del proveedor de DNS para ACME es independiente
		// del proveedor utilizado para gestionar los registros normales
		// del Gateway.
		if cfg.ACMEDNSProvider == "none" {
			return nil, fmt.Errorf(
				"certificates.mode=force-acme requiere acme.dns_provider configurado",
			)
		}

		// Configuramos primero el proveedor DNS-01.
		//
		// Es importante realizar esta operación antes de NewDefault(),
		// porque CertMagic crea su configuración a partir de los valores
		// predeterminados existentes en ese momento.
		if err := configureACMEDNS(cfg); err != nil {
			return nil, fmt.Errorf(
				"configurando proveedor DNS-01 de ACME: %w",
				err,
			)
		}

		// Ahora creamos CertMagic con la configuración DNS-01 ya aplicada.
		certConfig := certmagic.NewDefault()

		// Guardamos el estado de CertMagic en el directorio configurado
		// para evitar perder certificados y metadatos entre reinicios.
		certConfig.Storage = &certmagic.FileStorage{
			Path: certDir,
		}

		manager.certConfig = certConfig
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-005", "Solicitando/gestionando certificado ACME para:")
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-006", "Nombre incluido en el certificado: %s", nodeDomain)
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-007", "Nombre incluido en el certificado: %s", wildcardDomain)

		if until := readACMEBackoff(cfg, nodeDomain); !until.IsZero() && time.Now().Before(until) {
			return nil, fmt.Errorf("ACME temporalmente limitado hasta %s; no se solicitará otro certificado antes", until.Format(time.RFC3339))
		}

		// ManageSync gestiona el certificado de forma síncrona.
		//
		// Esto incluye las operaciones necesarias para obtener o renovar
		// el certificado mediante ACME.
		if err := certConfig.ManageSync(
			ctx,
			domainNames,
		); err != nil {
			storeACMEBackoff(cfg, nodeDomain, err)
			return nil, fmt.Errorf(
				"ACME obligatorio y no se pudo gestionar el certificado: %w",
				err,
			)
		}

		storeACMEBackoff(cfg, nodeDomain, nil)
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-008", "Certificado ACME gestionado correctamente")

		return manager, nil
	}
	diagnostics.

		// -------------------------------------------------------------------------
		// AUTO
		// -------------------------------------------------------------------------
		Emit( // En modo automático se intenta utilizar ACME siempre que exista
			// un proveedor configurado para ACME DNS-01.
			//
			// El proveedor DNS utilizado por ACME se controla exclusivamente
			// mediante acme.dns_provider.
			//
			// dns.provider no participa en esta decisión.
			"gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-009", "certificates.mode=auto: intentando utilizar ACME cuando sea posible")

	if cfg.ACMEDNSProvider != "none" {

		// Configuramos el proveedor DNS-01 seleccionado para ACME
		// antes de crear la configuración de CertMagic.
		if err := configureACMEDNS(cfg); err != nil {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-CERTIFICATES-010", "configurando proveedor DNS para ACME: %v", err)
		} else {
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-011", "Proveedor ACME DNS configurado: %s", cfg.ACMEDNSProvider)

			// NewDefault se ejecuta después de configurar DNS-01.
			// Así, el emisor ACME utiliza el proveedor DNS seleccionado
			// y no la configuración predeterminada anterior.
			certConfig := certmagic.NewDefault()

			// Guardamos el estado de CertMagic en el directorio configurado.
			certConfig.Storage = &certmagic.FileStorage{
				Path: certDir,
			}

			manager.certConfig = certConfig
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-012", "Solicitando/gestionando certificado ACME para:")
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-013", "Nombre incluido en el certificado: %s", nodeDomain)
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-014",

				// Antes de ordenar otro certificado, consultar el circuit breaker.
				// Si hay un HTTP 429 todavía vigente, reutilizamos ambos certificados
				// guardados (si existen) o servimos temporalmente la CA local.
				"Nombre incluido en el certificado: %s", wildcardDomain,
			)

			until := readACMEBackoff(cfg, nodeDomain)
			if !until.IsZero() && time.Now().Before(until) {
				diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-015", "evitando nueva solicitud para %s: límite hasta %s", nodeDomain, until.Format(time.RFC3339))
				if err := usableCachedACME(func(name string) error {
					_, e := certConfig.CacheManagedCertificate(ctx, name)
					return e
				}, nodeDomain, wildcardDomain); err == nil {
					diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-016", "par de certificados almacenados restaurado (principal + wildcard)")
					return manager, nil
				}
				return manager.loadLocalFallback(cfg, nodeDomain)
			}
			// ManageSync utiliza primero la caché de CertMagic. El refresco
			// automático sólo intentará una nueva emisión cuando corresponda.
			if err := certConfig.ManageSync(ctx, domainNames); err != nil {
				diagnostics.Emit("gateway", diagnostics.Warn, "HGW-CERTIFICATES-017", "gestión ACME no completada: %v", err)
				storeACMEBackoff(cfg, nodeDomain, err)
				if cacheErr := usableCachedACME(func(name string) error {
					_, e := certConfig.CacheManagedCertificate(ctx, name)
					return e
				}, nodeDomain, wildcardDomain); cacheErr == nil {
					diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-018", "certificado anterior principal y wildcard reutilizados")
					return manager, nil
				} else {
					diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-019", "par almacenado no disponible: %v", cacheErr)
				}
				diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-020", "utilizando CA local hasta el próximo reintento permitido")
				return manager.loadLocalFallback(cfg, nodeDomain)
			}
			storeACMEBackoff(cfg, nodeDomain, nil)
			diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-021", "Certificado ACME gestionado correctamente")

			return manager, nil
		}
	} else {
		diagnostics.
			// acme.dns_provider=none significa que no se ha configurado
			// ningún proveedor DNS para el desafío ACME DNS-01.
			//
			// Esto no afecta al proveedor DNS utilizado por el Gateway.
			Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-022", "acme.dns_provider=none: ACME DNS-01 no disponible")
	}
	diagnostics.

		// -------------------------------------------------------------------------
		// FALLBACK LOCAL
		// -------------------------------------------------------------------------
		Emit( // Si ACME no está disponible o no se ha configurado un proveedor
			// DNS para ACME, se utiliza la CA local como mecanismo alternativo.
			"gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-023", "Utilizando CA local como fallback")

	return manager.loadLocalFallback(
		cfg,
		nodeDomain,
	)
}

// configureACMEDNS configura CertMagic para utilizar el proveedor
// seleccionado en acme.dns_provider para el desafío ACME DNS-01.
//
// DNS-01 permite demostrar el control sobre un dominio creando
// temporalmente los registros DNS necesarios para ACME.
//
// Actualmente se soporta:
//
//   - technitium
//   - none
//
// "none" no configura ningún proveedor y es gestionado antes de
// llamar a esta función.
//
// dns.provider no interviene en esta selección.
func configureACMEDNS(
	cfg Config,
) error {

	switch cfg.ACMEDNSProvider {

	case "technitium":
		return configureTechnitiumACMEDNS(cfg)

	case "cloudflare":
		return configureCloudflareACMEDNS(cfg)

	case "none":
		return fmt.Errorf(
			"acme.dns_provider=none no proporciona un proveedor DNS para ACME",
		)

	default:
		return fmt.Errorf(
			"proveedor DNS ACME no soportado: %s",
			cfg.ACMEDNSProvider,
		)
	}
}

// configureTechnitiumACMEDNS configura CertMagic para utilizar
// Technitium como proveedor DNS para el desafío ACME DNS-01.
//
// La activación de Technitium para ACME depende de acme.dns_provider.
//
// La activación de Technitium para ACME se determina exclusivamente
// mediante:
//
//	acme.dns_provider=technitium
func configureTechnitiumACMEDNS(
	cfg Config,
) error {

	// Comprobamos que existe una URL para acceder a la API de Technitium.
	if cfg.ProviderDNSURL == "" {
		return fmt.Errorf(
			"dns.technitium.url está vacío; no se puede configurar ACME DNS-01",
		)
	}

	// Comprobamos que existe un token para autenticar las operaciones DNS.
	if cfg.ProviderDNSToken == "" {
		return fmt.Errorf(
			"dns.technitium.token está vacío; no se puede configurar ACME DNS-01",
		)
	}

	// Creamos el proveedor Technitium utilizando la URL del servidor
	// DNS y el token API configurados.
	provider := &technitium.Provider{
		ServerURL: cfg.ProviderDNSURL,
		APIToken:  cfg.ProviderDNSToken,
	}

	// Registramos Technitium como proveedor DNS-01 de CertMagic.
	certmagic.DefaultACME.DNS01Solver = &certmagic.DNS01Solver{
		DNSManager: certmagic.DNSManager{
			DNSProvider: provider,
			Resolvers: []string{
				"1.1.1.1:53",
				"8.8.8.8:53",
			},
		},
	}

	// El Gateway utilizará exclusivamente DNS-01 para ACME.
	//
	// No se utilizarán los desafíos HTTP-01 ni TLS-ALPN-01.
	certmagic.DefaultACME.DisableHTTPChallenge = true
	certmagic.DefaultACME.DisableTLSALPNChallenge = true
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-024", "ACME configurado para utilizar DNS-01 con Technitium")

	return nil
}

// loadLocalFallback carga el certificado local y lo registra dentro
// del certificateManager.
//
// Esta función se utiliza cuando el modo automático no puede utilizar
// ACME o cuando el proveedor DNS de ACME no está disponible.
func (m *certificateManager) loadLocalFallback(
	cfg Config,
	nodeDomain string,
) (*certificateManager, error) {

	// Carga o genera el certificado TLS local correspondiente al nodo.
	localTLSCertificate, err := loadLocalCertificate(
		cfg,
		nodeDomain,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"cargando certificado local de fallback: %w",
			err,
		)
	}

	// Guardamos el certificado local dentro del gestor.
	m.localTLSCertificate = localTLSCertificate
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CERTIFICATES-025", "Certificado TLS local preparado correctamente")

	return m, nil
}

// loadLocalCertificate prepara el certificado TLS local completo.
//
// Internamente:
//
//  1. Obtiene la ubicación persistente de la CA local.
//  2. Carga o genera la CA.
//  3. Obtiene la ubicación del certificado del nodo.
//  4. Carga, valida, renueva o genera el certificado local.
//
// La persistencia de la CA y del certificado permite que el Gateway
// pueda funcionar incluso sin conexión con Headscale, Technitium
// o Internet.
func loadLocalCertificate(
	cfg Config,
	nodeDomain string,
) (*tls.Certificate, error) {

	// Crea la referencia a la CA local asociada al dominio del nodo.
	ca := newLocalCA(
		cfg.DataDir,
		nodeDomain,
	)

	// Carga la CA existente o la genera si todavía no existe.
	//
	// Se utiliza context.Background() porque las operaciones de la CA
	// local no dependen de una conexión externa que necesite cancelación.
	caCert, caKey, err := ensureLocalCA(
		context.Background(),
		cfg,
		ca,
	)
	if err != nil {
		return nil, err
	}

	// Crea la referencia al certificado TLS del nodo.
	localCert := newLocalCertificate(
		cfg.DataDir,
		nodeDomain,
	)

	// Carga, valida, renueva o genera el certificado TLS local.
	//
	// El certificado queda firmado por la CA local obtenida anteriormente.
	tlsCertificate, _, err := ensureLocalCertificate(
		context.Background(),
		cfg,
		caCert,
		caKey,
		nodeDomain,
		localCert,
	)
	if err != nil {
		return nil, err
	}

	return tlsCertificate, nil
}

// TLSConfig devuelve la configuración TLS que debe utilizar el
// servidor HTTP.
//
// El certificado servido depende del modo:
//
//   - force-local:
//     utiliza siempre el certificado local.
//
//   - force-acme:
//     utiliza la configuración TLS proporcionada por CertMagic.
//
//   - auto:
//     utiliza el certificado local cuando está disponible;
//     de lo contrario utiliza CertMagic.
//
// El servidor HTTP utiliza posteriormente esta configuración
// independientemente de si la conexión procede de LAN o de tsnet.
func (m *certificateManager) TLSConfig() *tls.Config {

	switch m.mode {

	case "force-local":
		return m.localTLSConfig()

	case "force-acme":
		return m.certConfig.TLSConfig()

	case "auto":
		// Cuando existe un certificado local preparado, se utiliza
		// directamente para servir HTTPS.
		if m.localTLSCertificate != nil {
			return m.localTLSConfig()
		}

		// Mantiene separados los certificados DNS públicos y los de IP local.
		return withLocalAddressCertificate(m.certConfig.TLSConfig(), m.directTLSCertificate)

	default:
		// Este caso no debería alcanzarse porque validateConfig()
		// comprueba previamente que el modo de certificado sea válido.
		return &tls.Config{}
	}
}

// localTLSConfig crea la configuración TLS utilizada cuando el Gateway
// sirve el certificado generado por la CA local.
//
// El certificado se obtiene previamente mediante loadLocalCertificate()
// y se inserta directamente en tls.Config.
func (m *certificateManager) localTLSConfig() *tls.Config {

	return &tls.Config{
		// TLS 1.2 es la versión mínima aceptada por el Gateway.
		MinVersion: tls.VersionTLS12,

		// Certificado y clave privada que utilizará el servidor HTTPS.
		Certificates: []tls.Certificate{
			*m.localTLSCertificate,
		},
	}
}

func withLocalAddressCertificate(base *tls.Config, local *tls.Certificate) *tls.Config {
	config := base.Clone()
	original := config.GetCertificate
	config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// TLS clients normally omit SNI for IP literals.
		if local != nil && (hello.ServerName == "" || hello.ServerName == "localhost" || net.ParseIP(hello.ServerName) != nil) {
			return local, nil
		}
		if original != nil {
			return original(hello)
		}
		return nil, fmt.Errorf("certificado DNS no disponible")
	}
	return config
}
