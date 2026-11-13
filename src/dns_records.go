package main

import

// Manejo conservador de registros DNS propiedad del Gateway.
"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"context"
	"fmt"

	"strings"
)

// managedDNSCommentPrefix identifica los registros DNS creados y
// gestionados por este Gateway.
//
// El comentario completo almacenado en el servidor DNS incluye además
// el dominio del nodo, permitiendo distinguir los registros pertenecientes
// a diferentes instancias/nodos del Gateway.
const managedDNSCommentPrefix = "managed-by=headscale-gateway"

// managedDNSCommentForNode construye el comentario que se almacena
// en los registros DNS gestionados por este Gateway.
//
// El formato resultante es:
//
//	managed-by=headscale-gateway;node=equipo-1.intra.example.net
//
// Este comentario se utiliza posteriormente para identificar de forma
// segura qué registros puede modificar o eliminar el Gateway.
func managedDNSCommentForNode(nodeDomain string) string {
	return managedDNSCommentPrefix + ";node=" + nodeDomain
}

// ensureARecord garantiza que exista exactamente el registro A
// gestionado por este Gateway para un dominio e IP determinados.
//
// Esta función pertenece a la lógica común del Gateway y, por tanto,
// no conoce qué servidor DNS existe detrás de DNSProvider.
//
// La función tiene especial cuidado de no modificar registros A
// externos que no estén marcados con managedComment.
//
// Comportamiento:
//
//   - Más de un registro gestionado -> error.
//   - Ningún registro gestionado + A externo -> no modifica nada.
//   - Ningún registro A -> crea uno gestionado.
//   - Un registro gestionado con la IP correcta -> no hace nada.
//   - Un registro gestionado con otra IP -> actualiza únicamente
//     ese registro.
func ensureARecord(
	ctx context.Context,
	dns DNSProvider,
	zone string,
	domain string,
	ip string,
	managedComment string,
) error {

	// Obtiene los registros actuales del dominio a través del
	// proveedor DNS.
	records, err := dns.GetRecords(
		ctx,
		domain,
		zone,
		false,
	)
	if err != nil {
		return fmt.Errorf(
			"consultando %s: %w",
			domain,
			err,
		)
	}

	// managedRecords contiene exclusivamente los registros A cuyo
	// comentario coincide exactamente con el comentario gestionado.
	var managedRecords []*DNSRecord

	// externalRecord indica que existe al menos un registro A
	// que no pertenece a este Gateway.
	var externalRecord bool

	// Analiza todos los registros devueltos por el proveedor.
	for i := range records {
		record := &records[i]

		// Solamente interesan los registros de tipo A.
		if record.Type != "A" {
			continue
		}

		// Limpia espacios del comentario antes de compararlo.
		recordComment := strings.TrimSpace(
			record.Comments,
		)

		// Un comentario exactamente igual identifica un registro
		// gestionado por este Gateway.
		if recordComment == managedComment {
			managedRecords = append(
				managedRecords,
				record,
			)

			continue
		}

		// Cualquier otro registro A se considera externo.
		externalRecord = true
	}

	// ------------------------------------------------------------
	// Protección contra duplicados gestionados.
	// ------------------------------------------------------------

	// El diseño espera como máximo un registro gestionado por dominio.
	//
	// Si existen varios, se considera una inconsistencia y no se
	// intenta resolver automáticamente para evitar modificaciones
	// ambiguas.
	if len(managedRecords) > 1 {
		return fmt.Errorf(
			"inconsistencia DNS: existen %d registros A gestionados por este gateway para %s",
			len(managedRecords),
			domain,
		)
	}

	// ------------------------------------------------------------
	// No existe registro gestionado.
	// ------------------------------------------------------------

	if len(managedRecords) == 0 {

		// Si ya existe un registro A externo, no se sobrescribe.
		//
		// Esto protege los registros DNS que hayan sido creados
		// manualmente o por otra aplicación.
		if externalRecord {
			diagnostics.Emit("gateway", diagnostics.Warn, "HGW-DNS-RECORDS-001", "DNS externo detectado: %s A — no se modifica", domain)

			return fmt.Errorf("DNS %s: existe un registro A externo; no se puede confirmar la publicación", domain)
		}

		// --------------------------------------------------------
		// No existe ningún A.
		// Creamos nuestro registro.
		// --------------------------------------------------------

		// TTL de 300 segundos para los registros creados.
		const ttl = 300

		// Solicita al proveedor DNS la creación del registro.
		if err := dns.AddARecord(
			ctx,
			zone,
			domain,
			ip,
			ttl,
			managedComment,
		); err != nil {
			return fmt.Errorf(
				"creando A %s -> %s: %w",
				domain,
				ip,
				err,
			)
		}
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-RECORDS-002", "DNS creado: %s A %s", domain,
			ip,
		)

		return nil
	}

	// ------------------------------------------------------------
	// Existe exactamente un registro gestionado.
	// ------------------------------------------------------------

	managedRecord := managedRecords[0]

	// Obtiene la IP actualmente almacenada.
	currentIP := strings.TrimSpace(
		managedRecord.IP,
	)

	// ------------------------------------------------------------
	// La IP ya es correcta.
	// ------------------------------------------------------------

	// No se realiza ninguna modificación si el registro ya apunta
	// a la IP Tailscale actual.
	if currentIP == ip {
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-RECORDS-003", "DNS OK: %s A %s", domain,
			ip,
		)

		return nil
	}

	// ------------------------------------------------------------
	// Actualizamos exclusivamente nuestro registro.
	// ------------------------------------------------------------

	// TTL de 300 segundos, igual que en la creación.
	const ttl = 300

	// Actualiza exclusivamente el registro que ya estaba
	// identificado como gestionado por este Gateway.
	if err := dns.UpdateARecord(
		ctx,
		zone,
		domain,
		currentIP,
		ip,
		ttl,
		managedComment,
	); err != nil {
		return fmt.Errorf(
			"actualizando A %s: %s -> %s: %w",
			domain,
			currentIP,
			ip,
			err,
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-DNS-RECORDS-004", "DNS actualizado: %s A %s -> %s", domain,
		currentIP,
		ip,
	)

	return nil
}

// isGatewayDomain determina si un dominio pertenece al namespace
// DNS de un nodo concreto del Gateway.
//
// Devuelve true cuando:
//
//	domain == nodeDomain
//
// o cuando domain es un subdominio directo o descendiente de:
//
//	.<nodeDomain>
//
// Antes de comparar, ambos valores se normalizan eliminando
// espacios, puntos finales y diferencias de mayúsculas/minúsculas.
func isGatewayDomain(
	domain string,
	nodeDomain string,
) bool {

	// Normaliza el dominio recibido.
	domain = strings.TrimSuffix(
		strings.ToLower(
			strings.TrimSpace(domain),
		),
		".",
	)

	// Normaliza el dominio del nodo.
	nodeDomain = strings.TrimSuffix(
		strings.ToLower(
			strings.TrimSpace(nodeDomain),
		),
		".",
	)

	// Nunca se aceptan espacios de nombres vacíos. Esta protección evita
	// considerar gestionado un registro externo durante una limpieza.
	if domain == "" || nodeDomain == "" {
		return false
	}

	// El propio dominio del nodo pertenece al namespace.
	if domain == nodeDomain {
		return true
	}

	// También pertenecen todos los subdominios del nodo.
	return strings.HasSuffix(
		domain,
		"."+nodeDomain,
	)
}
