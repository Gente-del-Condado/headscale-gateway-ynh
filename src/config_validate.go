package main

import

// Validación de la configuración de DNS, TLS, proxy y el listener de administración.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/admin"

	"net"
	"net/url"
	"strconv"
	"strings"
)

func validateConfig(cfg Config) error {
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-CONFIG-VALIDATE-001",

		// -------------------------------------------------------------------------
		// Configuración general
		// -------------------------------------------------------------------------
		"Validando configuración...")

	// El dominio base es necesario para construir el dominio del nodo
	// y los dominios de los servicios.
	if cfg.BaseDomain == "" {
		return fmt.Errorf(
			"domain.base_domain no definido",
		)
	}

	// data_dir es necesario para almacenar estado persistente,
	// certificados, identidad, etc.
	if cfg.DataDir == "" {
		return fmt.Errorf(
			"data_dir no puede estar vacío",
		)
	}

	// -------------------------------------------------------------------------
	// Proveedores DNS
	// -------------------------------------------------------------------------

	// Proveedor utilizado para gestionar los registros DNS del Gateway.
	if !supportedOption(supportedDNSProviders, cfg.DNSProvider) {
		return fmt.Errorf("dns.provider inválido %q; valores permitidos: %s", cfg.DNSProvider, strings.Join(supportedDNSProviders, ", "))
	}

	// Proveedor utilizado exclusivamente para los challenges
	// DNS-01 de ACME.
	if !supportedOption(supportedACMEDNSProviders, cfg.ACMEDNSProvider) {
		return fmt.Errorf("acme.dns_provider inválido %q; valores permitidos: %s", cfg.ACMEDNSProvider, strings.Join(supportedACMEDNSProviders, ", "))
	}

	// -------------------------------------------------------------------------
	// Certificados
	// -------------------------------------------------------------------------

	// Solo se permiten los tres modos definidos por el Gateway.
	if !supportedOption(supportedCertificateModes, cfg.CertificateMode) {
		return fmt.Errorf("certificates.mode inválido %q; valores permitidos: %s", cfg.CertificateMode, strings.Join(supportedCertificateModes, ", "))
	}

	// acme.email es necesario siempre que el modo pueda utilizar ACME.
	//
	// force-local es la única excepción porque no necesita ACME.
	if cfg.CertificateMode != "force-local" &&
		cfg.ACMEEmail == "" {

		return fmt.Errorf(
			"acme.email no definido; es obligatorio con certificates.mode=%s",
			cfg.CertificateMode,
		)
	}

	// El periodo de renovación debe ser positivo.
	if cfg.CertificateRenewBefore <= 0 {
		return fmt.Errorf(
			"certificates.renew_before debe ser mayor que cero",
		)
	}

	// La CA local debe tener una duración positiva.
	if cfg.LocalCAValidity <= 0 {
		return fmt.Errorf(
			"certificates.local_ca_validity debe ser mayor que cero",
		)
	}

	// El certificado local debe tener una duración positiva.
	if cfg.LocalCertificateValidity <= 0 {
		return fmt.Errorf(
			"certificates.local_certificate_validity debe ser mayor que cero",
		)
	}

	// El periodo de renovación debe comenzar antes de la fecha
	// de caducidad del certificado.
	if cfg.CertificateRenewBefore >= cfg.LocalCertificateValidity {
		return fmt.Errorf(
			"certificates.renew_before debe ser menor que certificates.local_certificate_validity",
		)
	}

	// -------------------------------------------------------------------------
	// Proveedor DNS
	// -------------------------------------------------------------------------

	// El proveedor DNS puede ser utilizado actualmente por dos
	// componentes independientes:
	//
	//   1. dns.provider
	//      Gestión de registros del Gateway.
	//
	//   2. ACME_dns.provider
	//      Challenges DNS-01 de CertMagic.
	//
	// Ambos pueden utilizar el mismo endpoint, zona y token.
	providerDNSRequired :=
		cfg.DNSProvider == "technitium" ||
			cfg.ACMEDNSProvider == "technitium"

	if providerDNSRequired {

		// La zona DNS es necesaria cuando alguna de las funcionalidades
		// utiliza Technitium.
		if cfg.DNSZone == "" {
			return fmt.Errorf(
				"dns.zone no definido; el proveedor DNS está requerido por la configuración actual",
			)
		}

		if cfg.ProviderDNSURL == "" {
			return fmt.Errorf(
				"dns.technitium.url no definido; el proveedor DNS está requerido por la configuración actual",
			)
		}

		if cfg.ProviderDNSToken == "" {
			return fmt.Errorf(
				"dns.technitium.token no definido; el proveedor DNS está requerido por la configuración actual",
			)
		}

		// Comprobamos que la URL tenga un esquema y host válidos.
		if err := validateURL(
			cfg.ProviderDNSURL,
			"dns.technitium.url",
		); err != nil {
			return err
		}
	}

	// -------------------------------------------------------------------------
	// Cloudflare requiere un identificador de zona y un token diferentes para
	// cada función: publicación DNS y validación ACME.
	if cfg.DNSProvider == "cloudflare" {
		if cfg.DNSZone == "" {
			return fmt.Errorf("dns.zone es obligatorio para delimitar los registros gestionados por Cloudflare")
		}
		if err := validateCloudflareCredentials("dns.cloudflare", cfg.CloudflareDNSZoneID, cfg.CloudflareDNSToken); err != nil {
			return err
		}
	}
	if cfg.ACMEDNSProvider == "cloudflare" {
		if err := validateCloudflareCredentials("acme.cloudflare", cfg.CloudflareACMEZoneID, cfg.CloudflareACMEToken); err != nil {
			return err
		}
	}

	// Headscale
	// -------------------------------------------------------------------------

	// Aunque Headscale pueda estar temporalmente caído durante la ejecución,
	// su URL debe estar configurada porque es necesaria para los intentos
	// de conexión de tsnet.
	if cfg.HeadscaleURL == "" {
		return fmt.Errorf(
			"headscale.url no definido",
		)
	}

	// Validamos que la URL de Headscale tenga formato HTTP/HTTPS
	// y un host válido.
	if err := validateURL(
		cfg.HeadscaleURL,
		"headscale.url",
	); err != nil {
		return err
	}

	// -------------------------------------------------------------------------
	// Listener local
	// -------------------------------------------------------------------------

	// La dirección LAN es obligatoria después de aplicar los valores
	// por defecto.
	if cfg.LocalListenAddr == "" {
		return fmt.Errorf(
			"server.listen_addr no puede estar vacío",
		)
	}

	// ResolveTCPAddr comprueba que la dirección tenga un formato
	// válido para un listener TCP.
	if _, err := net.ResolveTCPAddr(
		"tcp",
		cfg.LocalListenAddr,
	); err != nil {
		return fmt.Errorf(
			"server.listen_addr inválido %q: %w",
			cfg.LocalListenAddr,
			err,
		)
	}

	if cfg.CAHTTPEnabled {
		host, port, e := net.SplitHostPort(cfg.CAHTTPListenAddr)
		n, pe := strconv.Atoi(port)
		if e != nil || pe != nil || n < 1 || n > 65535 || (host != "" && host != "localhost" && net.ParseIP(host) == nil) {
			return fmt.Errorf("dirección HTTP de CA inválida: usa IP:puerto entre 1 y 65535")
		}
		if !cfg.AdminEnabled {
			return fmt.Errorf("publicar CA por HTTP requiere administración habilitada")
		}
		_, httpsPort, _ := net.SplitHostPort(cfg.LocalListenAddr)
		_, adminPort, _ := net.SplitHostPort(cfg.AdminListenAddr)
		if port == httpsPort || port == adminPort {
			return fmt.Errorf("el puerto HTTP de CA debe ser distinto de HTTPS y administración")
		}
	}
	// El mismo contrato HTTP protege el acceso directo y el reenviado:
	// loopback por defecto o proxies confiables declarados explícitamente.
	if cfg.AdminEnabled {
		options, err := loadStartupOptions()
		if err != nil {
			return err
		}
		var httpOptions admin.HTTPOptions
		listenAddr := cfg.AdminListenAddr
		if options != nil {
			listenAddr = options.ListenAddr
			httpOptions = options.HTTPOptions
		}
		if err := admin.ValidateHTTPListenAddress(listenAddr, httpOptions); err != nil {
			return err
		}

	}

	// El panel publicado usa exclusivamente HTTPS sobre el listener existente.
	// "tailnet" no se puede saltar falsificando X-Forwarded-For: se distingue
	// el servidor que aceptó la conexión (LAN frente a tsnet).
	if cfg.AdminDomainMode != "off" && cfg.AdminDomainMode != "tailnet" && cfg.AdminDomainMode != "all" {
		return fmt.Errorf("admin.domain_mode inválido: %q (off, tailnet o all)", cfg.AdminDomainMode)
	}
	if !validAdminServiceName(cfg.AdminSubdomain) {
		return fmt.Errorf("admin.subdomain debe ser una única etiqueta DNS válida de hasta 63 caracteres")
	}
	if cfg.AdminDomainMode != "off" {
		if !cfg.AdminEnabled {
			return fmt.Errorf("admin.domain_mode requiere admin.enabled=true")
		}
		if _, found := cfg.Services[cfg.AdminSubdomain]; found {
			return fmt.Errorf("admin.subdomain=%q ya pertenece a un servicio; elige otro nombre", cfg.AdminSubdomain)
		}
	}

	// -------------------------------------------------------------------------
	// Servicios
	// -------------------------------------------------------------------------

	// Cada servicio debe apuntar a un backend HTTP o HTTPS válido.
	for serviceName, backend := range cfg.Services {
		normalized := normalizeServiceName(serviceName)
		if normalized == "" {
			return fmt.Errorf("nombre de servicio inválido %q", serviceName)
		}
		for other := range cfg.Services {
			if other != serviceName && normalizeServiceName(other) == normalized {
				return fmt.Errorf("servicios %q y %q colisionan tras normalización", serviceName, other)
			}
		}
		opt := cfg.ServiceProxy[serviceName]
		if opt.InsecureSkipVerify && !strings.HasPrefix(backend, "https://") {
			return fmt.Errorf("%s: tls.insecure_skip_verify solo admite HTTPS", serviceName)
		}

		// Un servicio no puede tener un backend vacío.
		if backend == "" {
			return fmt.Errorf(
				"backend vacío para servicio %s",
				serviceName,
			)
		}

		// Parseamos la URL del backend.
		target, err := url.Parse(backend)
		if err != nil {
			return fmt.Errorf(
				"URL inválida para %s: %w",
				serviceName,
				err,
			)
		}

		// El reverse proxy solo admite backends HTTP o HTTPS.
		if target.Scheme != "http" &&
			target.Scheme != "https" {

			return fmt.Errorf(
				"backend %s debe usar http:// o https://",
				serviceName,
			)
		}

		// La URL debe contener un host.
		if target.Host == "" {
			return fmt.Errorf(
				"backend %s no tiene host",
				serviceName,
			)
		}
	}
	diagnostics.

		// -------------------------------------------------------------------------
		// Configuración validada
		// -------------------------------------------------------------------------
		Emit("gateway", diagnostics.InfoLevel, "HGW-CONFIG-VALIDATE-002", "Configuración validada correctamente (%d servicios)", len(cfg.Services))

	return nil
}

// validateURL comprueba que una URL utilizada por el Gateway
// tenga un formato compatible con los servicios que necesitan
// conectarse mediante HTTP o HTTPS.
//
// Actualmente se utiliza para validar:
//
//   - dns.technitium.url
//   - headscale.url
//
// No realiza ninguna conexión de red; únicamente valida
// sintácticamente el esquema y el host.
func validateURL(value, name string) error {

	// Parseamos la URL.
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf(
			"%s inválida: %w",
			name,
			err,
		)
	}

	// Solo se aceptan HTTP y HTTPS.
	if u.Scheme != "http" &&
		u.Scheme != "https" {

		return fmt.Errorf(
			"%s debe utilizar http:// o https://",
			name,
		)
	}

	// Una URL válida para estos servicios debe incluir un host.
	if u.Host == "" {
		return fmt.Errorf(
			"%s no tiene host",
			name,
		)
	}

	return nil
}
