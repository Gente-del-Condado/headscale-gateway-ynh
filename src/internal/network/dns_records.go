package network

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
)

// MagicDNSRecord utiliza solo A/AAAA en el formato admitido por extra_records_path.
type MagicDNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// BuildMagicDNSRecords exporta de forma determinista sin modificar Headscale
// ni registros DNS manuales. Recibe únicamente servicios ya habilitados.
func BuildMagicDNSRecords(domain string, ips []string, serviceNames []string, adminName string) ([]MagicDNSRecord, error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if len(domain) > 240 || len(domain) == 0 {
		return nil, errors.New("dominio base inválido")
	}
	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if !validDNSLabel(label) {
			return nil, errors.New("dominio base inválido")
		}
	}
	unique := make(map[string]bool)
	for _, name := range serviceNames {
		if len(name)+len(domain)+1 > 253 || !validDNSLabel(name) {
			return nil, fmt.Errorf("servicio DNS inválido: %q", name)
		}
		unique[name] = true
	}
	if adminName != "" {
		if len(adminName)+len(domain)+1 > 253 || !validDNSLabel(adminName) {
			return nil, errors.New("subdominio administrativo inválido")
		}
		unique[adminName] = true
	}
	var addresses []netip.Addr
	seen := make(map[netip.Addr]bool)
	v4 := netip.MustParsePrefix("100.64.0.0/10")
	v6 := netip.MustParsePrefix("fd7a:115c:a1e0::/48")
	for _, candidate := range ips {
		ip, err := netip.ParseAddr(candidate)
		if err != nil || (!v4.Contains(ip) && !v6.Contains(ip)) {
			continue
		}
		if !seen[ip] {
			seen[ip] = true
			addresses = append(addresses, ip)
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("el Gateway no tiene una IP Tailscale activa")
	}
	names := make([]string, 0, len(unique))
	for n := range unique {
		names = append(names, n)
	}
	sort.Strings(names)
	sort.Slice(addresses, func(i, j int) bool { return addresses[i].Less(addresses[j]) })
	result := make([]MagicDNSRecord, 0, len(names)*len(addresses))
	for _, n := range names {
		for _, ip := range addresses {
			typ := "AAAA"
			if ip.Is4() {
				typ = "A"
			}
			result = append(result, MagicDNSRecord{Name: n + "." + domain, Type: typ, Value: ip.String()})
		}
	}
	return result, nil
}
func validDNSLabel(label string) bool {
	if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}
