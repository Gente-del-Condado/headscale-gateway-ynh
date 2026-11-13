package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// cloudflareDNS implementa la gestión de registros A mediante la API v4.
// No comparte credenciales con el proveedor ACME DNS-01.
type cloudflareDNS struct {
	zoneID, token, allowedZone, apiURL string
	client                             *http.Client
}

const cloudflareAPIURL = "https://api.cloudflare.com/client/v4"

var cfZoneIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

func validateCloudflareCredentials(prefix, zoneID, token string) error {
	if !cfZoneIDPattern.MatchString(zoneID) {
		return fmt.Errorf("%s.zone_id debe contener 32 caracteres hexadecimales", prefix)
	}
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("%s.token es obligatorio", prefix)
	}
	return nil
}

func newCloudflareDNS(zoneID, token, allowedZone string) *cloudflareDNS {
	return &cloudflareDNS{zoneID: zoneID, token: token, allowedZone: normalizeCFName(allowedZone), apiURL: cloudflareAPIURL,
		client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}

func normalizeCFName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}
func withinCFZone(name, zone string) bool {
	name = normalizeCFName(name)
	zone = normalizeCFName(zone)
	return zone != "" && (name == zone || strings.HasSuffix(name, "."+zone))
}

// Cloudflare no permite publicar registros proxy para IP privadas de la VPN;
// siempre utilizamos proxied:false y nunca hacemos DNS público por accidente
// cuando el usuario selecciona Technitium en lugar de Cloudflare.
type cfRecord struct {
	ID      string `json:"id,omitempty"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Comment string `json:"comment,omitempty"`
	Proxied bool   `json:"proxied"`
}
type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func (d *cloudflareDNS) do(ctx context.Context, method, path string, query url.Values, body any) (cfEnvelope, error) {
	var result cfEnvelope
	if d == nil || !cfZoneIDPattern.MatchString(d.zoneID) || d.token == "" {
		return result, errors.New("Cloudflare sin credenciales válidas")
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return result, err
		}
		reader = bytes.NewReader(encoded)
	}
	endpoint := strings.TrimRight(d.apiURL, "/") + "/zones/" + url.PathEscape(d.zoneID) + "/dns_records" + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("Cloudflare: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("Cloudflare: respuesta inválida (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !result.Success {
		message := "operación rechazada"
		if len(result.Errors) > 0 {
			message = fmt.Sprintf("código %d: %s", result.Errors[0].Code, result.Errors[0].Message)
		}
		return result, fmt.Errorf("Cloudflare HTTP %d: %s", resp.StatusCode, message)
	}
	return result, nil
}

func (d *cloudflareDNS) listRecords(ctx context.Context, recordType, exactName string) ([]cfRecord, error) {
	var records []cfRecord
	for page := 1; page <= 1000; page++ {
		query := url.Values{"type": {recordType}, "page": {strconv.Itoa(page)}, "per_page": {"100"}}
		if exactName != "" {
			query.Set("name", exactName)
		}
		env, err := d.do(ctx, http.MethodGet, "", query, nil)
		if err != nil {
			return nil, err
		}
		var part []cfRecord
		if err := json.Unmarshal(env.Result, &part); err != nil {
			return nil, fmt.Errorf("Cloudflare: registros inválidos: %w", err)
		}
		records = append(records, part...)
		if env.ResultInfo.TotalPages <= page {
			return records, nil
		}
	}
	return nil, errors.New("Cloudflare: paginación de registros excede el límite de seguridad")
}

func (d *cloudflareDNS) create(ctx context.Context, record cfRecord) (cfRecord, error) {
	env, err := d.do(ctx, http.MethodPost, "", nil, record)
	if err != nil {
		return cfRecord{}, err
	}
	var created cfRecord
	if err := json.Unmarshal(env.Result, &created); err != nil {
		return cfRecord{}, err
	}
	return created, nil
}
func (d *cloudflareDNS) patch(ctx context.Context, id string, record cfRecord) error {
	if id == "" {
		return errors.New("Cloudflare: id de registro vacío")
	}
	_, err := d.do(ctx, http.MethodPatch, "/"+url.PathEscape(id), nil, record)
	return err
}
func (d *cloudflareDNS) remove(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("Cloudflare: id de registro vacío")
	}
	_, err := d.do(ctx, http.MethodDelete, "/"+url.PathEscape(id), nil, nil)
	return err
}

func (d *cloudflareDNS) validateName(zone, name string) error {
	if !withinCFZone(name, d.allowedZone) {
		return fmt.Errorf("Cloudflare: %s fuera de dns.zone=%s", name, d.allowedZone)
	}
	if zone != "" && !withinCFZone(name, zone) {
		return fmt.Errorf("Cloudflare: %s fuera de la zona de Gateway %s", name, zone)
	}
	return nil
}

func (d *cloudflareDNS) GetRecords(ctx context.Context, domain, zone string, listZone bool) ([]DNSRecord, error) {
	if err := d.validateName(zone, domain); err != nil {
		return nil, err
	}
	exact := ""
	if !listZone {
		exact = normalizeCFName(domain)
	}
	found, err := d.listRecords(ctx, "A", exact)
	if err != nil {
		return nil, err
	}
	out := make([]DNSRecord, 0, len(found))
	for _, r := range found {
		if (listZone && !withinCFZone(r.Name, domain)) || (!listZone && normalizeCFName(r.Name) != normalizeCFName(domain)) {
			continue
		}
		out = append(out, DNSRecord{Name: r.Name, Type: r.Type, IP: r.Content, TTL: r.TTL, Comments: r.Comment})
	}
	return out, nil
}

func (d *cloudflareDNS) AddARecord(ctx context.Context, zone, domain, ip string, ttl int, comments string) error {
	if err := d.validateName(zone, domain); err != nil {
		return err
	}
	if net.ParseIP(ip) == nil || net.ParseIP(ip).To4() == nil {
		return fmt.Errorf("Cloudflare: IP A inválida")
	}
	if ttl < 60 || ttl > 86400 {
		return fmt.Errorf("Cloudflare: TTL debe estar entre 60 y 86400")
	}
	if !strings.HasPrefix(comments, managedDNSCommentPrefix+";") {
		return errors.New("Cloudflare: falta marca de propiedad del gateway")
	}
	_, err := d.create(ctx, cfRecord{Name: normalizeCFName(domain), Type: "A", Content: ip, TTL: ttl, Comment: comments, Proxied: false})
	return err
}

// Para editar o borrar, se resuelve ID por nombre, IP y comentario de propiedad.
// Nunca se modifica un A externo que tenga el mismo nombre.
func (d *cloudflareDNS) ownedRecord(ctx context.Context, zone, domain, ip, comment string) (cfRecord, error) {
	if err := d.validateName(zone, domain); err != nil {
		return cfRecord{}, err
	}
	records, err := d.listRecords(ctx, "A", normalizeCFName(domain))
	if err != nil {
		return cfRecord{}, err
	}
	var match cfRecord
	for _, r := range records {
		if normalizeCFName(r.Name) != normalizeCFName(domain) || r.Content != ip || r.Comment != comment {
			continue
		}
		if match.ID != "" {
			return cfRecord{}, fmt.Errorf("Cloudflare: múltiples registros gestionados para %s", domain)
		}
		match = r
	}
	if match.ID == "" {
		return cfRecord{}, fmt.Errorf("Cloudflare: no se encontró registro gestionado %s (%s)", domain, ip)
	}
	return match, nil
}
func (d *cloudflareDNS) UpdateARecord(ctx context.Context, zone, domain, currentIP, newIP string, ttl int, comments string) error {
	if net.ParseIP(newIP) == nil || net.ParseIP(newIP).To4() == nil {
		return errors.New("Cloudflare: IPv4 inválida")
	}
	if ttl < 60 || ttl > 86400 {
		return errors.New("Cloudflare: TTL fuera de rango")
	}
	old, err := d.ownedRecord(ctx, zone, domain, currentIP, comments)
	if err != nil {
		return err
	}
	return d.patch(ctx, old.ID, cfRecord{Name: normalizeCFName(domain), Type: "A", Content: newIP, TTL: ttl, Comment: comments, Proxied: false})
}
func (d *cloudflareDNS) DeleteARecord(ctx context.Context, zone, domain, ip string) error {
	if err := d.validateName(zone, domain); err != nil {
		return err
	}
	records, err := d.listRecords(ctx, "A", normalizeCFName(domain))
	if err != nil {
		return err
	}
	var match cfRecord
	for _, r := range records {
		if normalizeCFName(r.Name) != normalizeCFName(domain) || r.Content != ip || !strings.HasPrefix(r.Comment, managedDNSCommentPrefix+";node=") {
			continue
		}
		node := strings.TrimPrefix(r.Comment, managedDNSCommentPrefix+";node=")
		if !isGatewayDomain(domain, node) {
			continue
		}
		if match.ID != "" {
			return fmt.Errorf("Cloudflare: más de un A gestionado %s", domain)
		}
		match = r
	}
	if match.ID == "" {
		return fmt.Errorf("Cloudflare: A gestionado no encontrado %s", domain)
	}
	return d.remove(ctx, match.ID)
}
