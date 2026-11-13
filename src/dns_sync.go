package main

import

// Publicación de DNS privado al confirmar la IP de tsnet.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"fmt"

	"net"
	"strings"
	"tailscale.com/tsnet"
)

// syncGatewayDNS sincroniza los registros A utilizados por el Gateway.
//
// Esta función contiene la lógica DNS común del Gateway.
//
// Importante:
//
// Esta función NO conoce Technitium ni ninguna otra implementación
// concreta. Recibe un DNSProvider y utiliza exclusivamente el contrato
// definido por esa interfaz.
//
// BaseDomain y DNSZone tienen responsabilidades diferentes:
//
//   - BaseDomain define el dominio base utilizado por el Gateway.
//   - DNSZone identifica la zona DNS utilizada por el proveedor.
//
// Aunque actualmente ambos valores pueden coincidir en Technitium,
// se mantienen separados para permitir que cada proveedor gestione
// su propia representación de zona.
//
// El dominio principal del nodo y cada uno de sus servicios deben
// apuntar a la dirección IPv4 Tailscale del Gateway.
//
// Además de crear o actualizar registros, realiza una limpieza
// de registros gestionados anteriormente que ya no sean necesarios.
//
// Los registros DNS externos no pertenecientes al Gateway se
// conservan intactos.
func syncGatewayDNS(
	ctx context.Context,
	cfg Config,
	tsServer *tsnet.Server,
	nodeDomain string,
	services map[string]string,
	dns DNSProvider,
) error {

	// La sincronización necesita una instancia activa de tsnet
	// para conocer la dirección IP Tailscale que debe publicarse.
	if tsServer == nil {
		return fmt.Errorf(
			"tsnet no está disponible para sincronizar DNS VPN",
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-SYNC-001",

		// Obtiene las direcciones Tailscale del nodo.
		//
		// Esta sincronización utiliza únicamente la dirección IPv4.
		"Sincronizando DNS del gateway...")

	ip4, _ := tsServer.TailscaleIPs()

	// Comprueba que exista una dirección válida.
	if !ip4.IsValid() {
		return fmt.Errorf(
			"tsnet no tiene una dirección IPv4 Tailscale válida",
		)
	}

	// El Gateway publica registros A, por lo que necesita una
	// dirección IPv4 y no una IPv6.
	if ip4.Is6() {
		return fmt.Errorf(
			"la dirección Tailscale obtenida no es IPv4: %s",
			ip4,
		)
	}

	// Convierte la dirección a la representación textual utilizada
	// por los registros DNS A.
	tailscaleIP := net.IP(
		ip4.AsSlice(),
	).String()
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-SYNC-002", "IP Tailscale del gateway: %s", tailscaleIP)

	// Genera la marca que identifica todos los registros gestionados
	// por esta instancia del Gateway.
	managedComment := managedDNSCommentForNode(
		nodeDomain,
	)

	// ------------------------------------------------------------
	// Lista de nombres DNS que ESTE Gateway debe gestionar.
	// ------------------------------------------------------------

	// expectedDomains contiene el conjunto de dominios que deben
	// permanecer presentes después de la sincronización.
	expectedDomains := make(
		map[string]struct{},
	)

	// Normaliza el dominio del nodo para utilizarlo como clave
	// de comparación.
	normalizedNodeDomain := strings.TrimSuffix(
		strings.ToLower(
			strings.TrimSpace(nodeDomain),
		),
		".",
	)

	expectedDomains[normalizedNodeDomain] = struct{}{}

	// Publicación opcional de admin.<nodo>.<dominio-base>. El certificado
	// wildcard ya cubre este nombre; no se emite ningún certificado adicional.
	// El modo "tailnet" mantiene el acceso restringido por el listener tsnet.
	if cfg.AdminEnabled && cfg.AdminDomainMode != "off" {
		adminHost := cfg.AdminSubdomain + "." + nodeDomain
		expectedDomains[adminHost] = struct{}{}
		if err := ensureARecord(ctx, dns, cfg.DNSZone, adminHost, tailscaleIP, managedComment); err != nil {
			return fmt.Errorf("DNS administrativo %s: %w", adminHost, err)
		}
	}

	// ------------------------------------------------------------
	// Dominio principal del nodo
	// ------------------------------------------------------------

	// Garantiza que el dominio principal del nodo apunte a la
	// dirección IPv4 Tailscale actual.
	if err := ensureARecord(
		ctx,
		dns,
		cfg.DNSZone,
		nodeDomain,
		tailscaleIP,
		managedComment,
	); err != nil {
		return err
	}

	// ------------------------------------------------------------
	// Servicios
	// ------------------------------------------------------------

	// Cada servicio obtiene un hostname bajo el dominio del nodo.
	for serviceName := range services {

		// Normaliza el nombre lógico del servicio antes de utilizarlo
		// como etiqueta DNS.
		serviceName = normalizeServiceName(
			serviceName,
		)

		// Un nombre que quede vacío después de normalizarse no puede
		// utilizarse para construir un hostname.
		if serviceName == "" {
			continue
		}

		// Construye el FQDN del servicio.
		//
		// Ejemplo:
		//
		//	home-assistant.equipo-1.intra.example.net
		serviceHost := serviceName + "." + nodeDomain

		// Normaliza el nombre utilizado como clave en expectedDomains
		// para que la comparación con los registros del proveedor
		// sea consistente.
		normalizedServiceHost := strings.TrimSuffix(
			strings.ToLower(
				strings.TrimSpace(serviceHost),
			),
			".",
		)

		expectedDomains[normalizedServiceHost] = struct{}{}

		// Garantiza que el registro A del servicio apunte a la IP
		// Tailscale actual.
		if err := ensureARecord(
			ctx,
			dns,
			cfg.DNSZone,
			serviceHost,
			tailscaleIP,
			managedComment,
		); err != nil {
			return err
		}
	}

	// ------------------------------------------------------------
	// Limpieza de registros gestionados que ya no son necesarios.
	// ------------------------------------------------------------

	// Elimina únicamente registros antiguos que hayan sido creados
	// por este Gateway y que ya no aparezcan en expectedDomains.
	if err := cleanupGatewayDNS(
		ctx,
		dns,
		cfg.DNSZone,
		nodeDomain,
		expectedDomains,
		managedComment,
	); err != nil {
		return err
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-SYNC-003", "DNS del gateway sincronizado correctamente")

	return nil
}
