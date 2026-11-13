package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/libdns"
)

// cloudflareDNS01Provider implements libdns.RecordAppender/RecordDeleter.
// A diferencia de dns_cloudflare.go, solo manipula TXT temporales ACME.
type cloudflareDNS01Provider struct{ client *cloudflareDNS }

var _ certmagic.DNSProvider = (*cloudflareDNS01Provider)(nil)

func (p *cloudflareDNS01Provider) AppendRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	output := make([]libdns.Record, 0, len(recs))
	for _, input := range recs {
		r := input.RR()
		if r.Type != "TXT" {
			return output, fmt.Errorf("Cloudflare DNS-01: tipo %s no admitido", r.Type)
		}
		name := normalizeCFName(libdns.AbsoluteName(r.Name, zone))
		if !withinCFZone(name, zone) || !strings.HasPrefix(name, "_acme-challenge.") && !strings.Contains(name, "._acme-challenge.") {
			return output, errors.New("Cloudflare DNS-01: solo se permiten nombres _acme-challenge dentro de la zona")
		}
		// CertMagic utiliza un valor TXT arbitrario: la conversión a RR produce
		// el valor plano de la librería, con las reglas de cita propias de libdns.
		txt, err := cloudflareTXT(input)
		if err != nil {
			return output, err
		}
		ttl := int(txt.TTL / time.Second)
		if ttl < 60 {
			ttl = 120
		}
		if ttl > 86400 {
			ttl = 86400
		}
		created, err := p.client.create(ctx, cfRecord{Name: name, Type: "TXT", Content: txt.Text, TTL: ttl, Proxied: false})
		if err != nil {
			return output, err
		}
		output = append(output, libdns.TXT{Name: r.Name, Text: txt.Text, TTL: time.Duration(created.TTL) * time.Second, ProviderData: created.ID})
	}
	return output, nil
}

func (p *cloudflareDNS01Provider) DeleteRecords(ctx context.Context, zone string, recs []libdns.Record) ([]libdns.Record, error) {
	output := make([]libdns.Record, 0, len(recs))
	for _, input := range recs {
		r := input.RR()
		if r.Type != "TXT" {
			return output, errors.New("Cloudflare DNS-01: solo pueden eliminarse TXT")
		}
		name := normalizeCFName(libdns.AbsoluteName(r.Name, zone))
		if !withinCFZone(name, zone) || !strings.HasPrefix(name, "_acme-challenge.") && !strings.Contains(name, "._acme-challenge.") {
			return output, errors.New("Cloudflare DNS-01: nombre fuera de la zona o no es desafío ACME")
		}
		txt, err := cloudflareTXT(input)
		if err != nil {
			return output, err
		}
		records, err := p.client.listRecords(ctx, "TXT", name)
		if err != nil {
			return output, err
		}
		id, _ := txt.ProviderData.(string)
		var matching []cfRecord
		for _, record := range records {
			if normalizeCFName(record.Name) != name || record.Content != txt.Text {
				continue
			}
			if id != "" && record.ID != id {
				continue
			}
			matching = append(matching, record)
		}
		if len(matching) == 0 {
			continue
		} // borrado idempotente
		if len(matching) != 1 {
			return output, fmt.Errorf("Cloudflare DNS-01: %d TXT idénticos para %s; no se borra ninguno", len(matching), name)
		}
		if err := p.client.remove(ctx, matching[0].ID); err != nil {
			return output, err
		}
		output = append(output, input)
	}
	return output, nil
}

func configureCloudflareACMEDNS(cfg Config) error {
	if err := validateCloudflareCredentials("acme.cloudflare", cfg.CloudflareACMEZoneID, cfg.CloudflareACMEToken); err != nil {
		return err
	}
	provider := &cloudflareDNS01Provider{client: newCloudflareDNS(cfg.CloudflareACMEZoneID, cfg.CloudflareACMEToken, "")}
	certmagic.DefaultACME.DNS01Solver = &certmagic.DNS01Solver{DNSManager: certmagic.DNSManager{
		DNSProvider: provider,
		Resolvers:   []string{"1.1.1.1:53", "8.8.8.8:53"},
	}}
	certmagic.DefaultACME.DisableHTTPChallenge = true
	certmagic.DefaultACME.DisableTLSALPNChallenge = true
	return nil
}

// También admite registros RR genéricos: libdns exige que los proveedores
// acepten cualquiera de las representaciones de Record.
func cloudflareTXT(record libdns.Record) (libdns.TXT, error) {
	if txt, ok := record.(libdns.TXT); ok {
		return txt, nil
	}
	parsed, err := record.RR().Parse()
	if err != nil {
		return libdns.TXT{}, fmt.Errorf("Cloudflare DNS-01: TXT inválido: %w", err)
	}
	txt, ok := parsed.(libdns.TXT)
	if !ok {
		return libdns.TXT{}, errors.New("Cloudflare DNS-01: registro TXT incompatible")
	}
	return txt, nil
}
