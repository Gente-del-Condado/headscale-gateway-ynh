package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// technitiumAPIResponse representa la estructura común de respuesta
// de la API HTTP de Technitium DNS.
//
// La estructura contiene únicamente los campos que necesita el
// Gateway para interpretar las respuestas de Technitium.
type technitiumAPIResponse struct {
	Status   string `json:"status"`
	Response struct {
		Records []technitiumRecord `json:"records"`
	} `json:"response"`
}

// technitiumRecord representa el formato de un registro DNS tal y
// como lo devuelve la API de Technitium.
//
// Este tipo pertenece exclusivamente al adaptador de Technitium.
//
// La lógica general del Gateway no debe trabajar directamente con
// este tipo, sino con DNSRecord.
type technitiumRecord struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	TTL      int    `json:"ttl"`
	Disabled bool   `json:"disabled"`
	RData    struct {
		IPAddress string `json:"ipAddress"`
	} `json:"rData"`
	Comments string `json:"comments"`
}

// technitiumDNS encapsula la información necesaria para comunicarse
// con la API HTTP de Technitium DNS.
//
// Este tipo implementará la interfaz DNSProvider.
//
// De esta forma, el resto del Gateway no necesita conocer cómo
// funciona internamente la API de Technitium.
type technitiumDNS struct {
	baseURL string
	token   string
	client  *http.Client
}

// newTechnitiumDNS crea un proveedor DNS basado en Technitium.
//
// Los valores de cfg.ProviderDNSURL y cfg.ProviderDNSToken proceden
// de dns.technitium.url y dns.technitium.token almacenados en SQLite.
//
// La URL configurada se normaliza eliminando las barras finales
// para poder concatenarla posteriormente con las rutas de la API.
//
// El cliente HTTP tiene un timeout global de 15 segundos para evitar
// que una operación contra Technitium quede bloqueada indefinidamente.
func newTechnitiumDNS(cfg Config) *technitiumDNS {
	return &technitiumDNS{
		baseURL: strings.TrimRight(
			cfg.ProviderDNSURL,
			"/",
		),
		token: cfg.ProviderDNSToken,
		client: &http.Client{
			Timeout:       15 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// request realiza una petición HTTP contra la API de Technitium.
//
// El token de autenticación se añade a los parámetros de la petición.
//
// La función se encarga de:
//
//   - construir la URL completa,
//   - crear la petición asociada al contexto,
//   - ejecutarla,
//   - leer la respuesta,
//   - comprobar el código HTTP,
//   - decodificar el JSON,
//   - comprobar el status devuelto por Technitium.
//
// Devuelve la respuesta API ya decodificada si la operación ha sido
// aceptada correctamente.
func (d *technitiumDNS) request(
	ctx context.Context,
	method string,
	path string,
	values url.Values,
) (*technitiumAPIResponse, error) {
	if values == nil {
		values = url.Values{}
	}

	values.Set("token", d.token)
	requestURL := d.baseURL + path + "?" + values.Encode()

	req, err := http.NewRequestWithContext(ctx, method, requestURL, nil)
	if err != nil {
		return nil, errors.New("URL de Technitium inválida")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		// Un url.Error incluye la consulta completa, con la credencial DNS.
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		return nil, errors.New("No se pudo conectar con Technitium")
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Technitium HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New("No se pudo leer la respuesta de Technitium")
	}

	var result technitiumAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, errors.New("Respuesta JSON inválida de Technitium")
	}

	if result.Status != "ok" {
		return nil, errors.New("Technitium ha rechazado la operación DNS")
	}

	return &result, nil
}

// GetRecords obtiene los registros DNS de un dominio mediante
// la API de Technitium.
//
// Esta función implementa la operación GetRecords de DNSProvider.
//
// El formato específico de Technitium se convierte posteriormente
// al modelo común DNSRecord utilizado por el Gateway.
func (d *technitiumDNS) GetRecords(
	ctx context.Context,
	domain string,
	zone string,
	listZone bool,
) ([]DNSRecord, error) {
	values := url.Values{}
	values.Set("domain", domain)
	values.Set("zone", zone)
	if listZone {
		values.Set("listZone", "true")
	}

	result, err := d.request(ctx, http.MethodGet, "/api/zones/records/get", values)
	if err != nil {
		return nil, err
	}

	records := make([]DNSRecord, 0, len(result.Response.Records))
	for _, record := range result.Response.Records {
		records = append(records, DNSRecord{
			Name:     record.Name,
			Type:     record.Type,
			TTL:      record.TTL,
			Disabled: record.Disabled,
			IP:       record.RData.IPAddress,
			Comments: record.Comments,
		})
	}
	return records, nil
}

// AddARecord crea un registro A en Technitium.
//
// Esta función implementa la operación AddARecord de DNSProvider.
func (d *technitiumDNS) AddARecord(
	ctx context.Context,
	zone string,
	domain string,
	ip string,
	ttl int,
	comments string,
) error {
	values := url.Values{}
	values.Set("domain", domain)
	values.Set("zone", zone)
	values.Set("type", "A")
	values.Set("ttl", fmt.Sprintf("%d", ttl))
	values.Set("ipAddress", ip)
	values.Set("comments", comments)

	if _, err := d.request(ctx, http.MethodGet, "/api/zones/records/add", values); err != nil {
		return err
	}
	return nil
}

// UpdateARecord actualiza un registro A existente en Technitium.
//
// currentIP identifica la dirección actualmente almacenada.
//
// newIP contiene la nueva dirección que debe utilizar el registro.
func (d *technitiumDNS) UpdateARecord(
	ctx context.Context,
	zone string,
	domain string,
	currentIP string,
	newIP string,
	ttl int,
	comments string,
) error {
	values := url.Values{}
	values.Set("domain", domain)
	values.Set("zone", zone)
	values.Set("type", "A")
	values.Set("ipAddress", currentIP)
	values.Set("newIpAddress", newIP)
	values.Set("ttl", fmt.Sprintf("%d", ttl))
	values.Set("comments", comments)

	if _, err := d.request(ctx, http.MethodGet, "/api/zones/records/update", values); err != nil {
		return err
	}
	return nil
}

// DeleteARecord elimina un registro A concreto de Technitium.
//
// La combinación de dominio, zona e IP identifica el registro
// que se solicita eliminar.
func (d *technitiumDNS) DeleteARecord(
	ctx context.Context,
	zone string,
	domain string,
	ip string,
) error {
	values := url.Values{}
	values.Set("domain", domain)
	values.Set("zone", zone)
	values.Set("type", "A")
	values.Set("ipAddress", ip)

	if _, err := d.request(ctx, http.MethodGet, "/api/zones/records/delete", values); err != nil {
		return err
	}
	return nil
}

// Comprobación en tiempo de compilación de que technitiumDNS
// implementa completamente la interfaz DNSProvider.
//
// Si en el futuro se modifica DNSProvider y Technitium deja de
// implementar alguno de sus métodos, el compilador señalará
// inmediatamente el problema.
var _ DNSProvider = (*technitiumDNS)(nil)
