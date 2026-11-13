package main

import

// Limpieza de registros antiguos sin afectar a otros administradores.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"fmt"

	"strings"
)

// cleanupGatewayDNS elimina registros A antiguos que hayan sido
// creados por ESTE Gateway pero que ya no formen parte de los
// dominios esperados.
//
// La función aplica varias protecciones antes de eliminar:
//
//  1. Solo procesa registros A.
//  2. El comentario debe coincidir exactamente con managedComment.
//  3. El dominio debe pertenecer al namespace del nodo.
//  4. El dominio no debe aparecer en expectedDomains.
//  5. El registro debe tener una IP válida.
//
// Los registros externos no gestionados por el Gateway nunca se
// eliminan mediante esta función.
func cleanupGatewayDNS(
	ctx context.Context,
	dns DNSProvider,
	zone string,
	nodeDomain string,
	expectedDomains map[string]struct{},
	managedComment string,
) error {
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-CLEANUP-001", "Comprobando registros DNS obsoletos del gateway %s...", nodeDomain)

	// Obtiene todos los registros de la zona asociada al dominio
	// del nodo para poder detectar registros gestionados que hayan
	// quedado obsoletos.
	records, err := dns.GetRecords(
		ctx,
		nodeDomain,
		zone,
		true,
	)
	if err != nil {
		return fmt.Errorf(
			"consultando registros DNS del nodo %s: %w",
			nodeDomain,
			err,
		)
	}

	// Analiza cada registro encontrado.
	for _, record := range records {

		// --------------------------------------------------------
		// Solo registros A.
		// --------------------------------------------------------

		if record.Type != "A" {
			continue
		}

		// --------------------------------------------------------
		// Solo registros pertenecientes a ESTE Gateway.
		// --------------------------------------------------------
		//
		// Esta comprobación es la principal protección contra
		// la eliminación accidental de registros externos.
		if strings.TrimSpace(record.Comments) != managedComment {
			continue
		}

		// --------------------------------------------------------
		// Normalizamos el nombre DNS.
		// --------------------------------------------------------

		domain := strings.TrimSuffix(
			strings.ToLower(
				strings.TrimSpace(record.Name),
			),
			".",
		)

		// --------------------------------------------------------
		// Segunda protección:
		// el registro debe pertenecer al namespace del nodo.
		// --------------------------------------------------------

		if !isGatewayDomain(
			domain,
			nodeDomain,
		) {
			continue
		}

		// --------------------------------------------------------
		// Si sigue siendo necesario, no lo eliminamos.
		// --------------------------------------------------------

		// expectedDomains contiene los nombres que el Gateway
		// considera actualmente necesarios.
		if _, expected := expectedDomains[domain]; expected {
			continue
		}

		// Obtiene la IP del registro que se pretende eliminar.
		ip := strings.TrimSpace(
			record.IP,
		)

		// Sin una IP válida no se intenta realizar una eliminación,
		// evitando una operación potencialmente ambigua.
		if ip == "" {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-DNS-CLEANUP-002", "DNS gestionado sin IP válida: %s — no se elimina", domain)

			continue
		}
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-CLEANUP-003", "DNS obsoleto detectado: %s A %s", domain,
			ip,
		)

		// Elimina exclusivamente el registro gestionado que ha sido
		// identificado como obsoleto.
		if err := dns.DeleteARecord(
			ctx,
			zone,
			domain,
			ip,
		); err != nil {
			return err
		}
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-CLEANUP-004", "Limpieza DNS del gateway completada")

	return nil
}
