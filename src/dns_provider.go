package main

import "context"

// DNSRecord representa la información DNS que necesita conocer
// el Gateway independientemente del servidor DNS utilizado.
//
// Es un modelo interno del Gateway.
//
// Cada proveedor DNS será responsable de convertir su propio
// formato de API a este modelo.
type DNSRecord struct {
	// Name contiene el nombre completo del registro DNS.
	//
	// Ejemplo:
	//
	//	home-assistant.equipo-1.intra.example.net
	Name string

	// Type contiene el tipo de registro DNS.
	//
	// Actualmente el Gateway utiliza principalmente registros A.
	Type string

	// TTL contiene el tiempo de vida del registro en segundos.
	TTL int

	// Disabled indica si el registro está deshabilitado
	// en el servidor DNS.
	Disabled bool

	// IP contiene la dirección IP asociada al registro.
	//
	// Para un registro A corresponde a la dirección IPv4.
	IP string

	// Comments contiene los comentarios asociados al registro.
	//
	// El Gateway utiliza este campo para identificar sus propios
	// registros gestionados.
	Comments string
}

// DNSProvider define las operaciones DNS que necesita el Gateway.
//
// Esta interfaz NO representa la API de un producto concreto.
//
// Por ejemplo, Technitium, PowerDNS u otro servidor DNS pueden
// implementar esta misma interfaz aunque sus APIs HTTP sean
// completamente diferentes.
//
// La lógica del Gateway trabajará únicamente con DNSProvider.
//
// El proveedor concreto será responsable de traducir estas
// operaciones a su propia API.
type DNSProvider interface {

	// GetRecords obtiene los registros DNS asociados a un dominio.
	//
	// domain es el nombre que se desea consultar.
	//
	// zone identifica la zona DNS correspondiente.
	//
	// listZone permite solicitar información adicional de la
	// zona cuando el proveedor lo soporte.
	GetRecords(
		ctx context.Context,
		domain string,
		zone string,
		listZone bool,
	) ([]DNSRecord, error)

	// AddARecord crea un registro DNS de tipo A.
	//
	// El proveedor debe encargarse de realizar la operación
	// utilizando su propia API.
	AddARecord(
		ctx context.Context,
		zone string,
		domain string,
		ip string,
		ttl int,
		comments string,
	) error

	// UpdateARecord actualiza la dirección IP de un registro A
	// que ya existe.
	//
	// currentIP identifica el registro que debe modificarse.
	//
	// newIP contiene la nueva dirección IP.
	UpdateARecord(
		ctx context.Context,
		zone string,
		domain string,
		currentIP string,
		newIP string,
		ttl int,
		comments string,
	) error

	// DeleteARecord elimina un registro A concreto.
	//
	// La combinación de dominio, zona e IP identifica el registro
	// que se desea eliminar.
	DeleteARecord(
		ctx context.Context,
		zone string,
		domain string,
		ip string,
	) error
}
