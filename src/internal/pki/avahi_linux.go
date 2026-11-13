//go:build linux && !android

package pki

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DiscoverLAN usa Avahi/DNS-SD instalado. Los anuncios solo aportan pistas
// no confiables de descubrimiento; no prueban identidad ni conceden aprobación.
func DiscoverLAN(ctx context.Context, ports ...int) ([]Service, error) {
	binary, e := exec.LookPath("avahi-browse")
	if e != nil {
		return DiscoverPortable(ctx, ports...)
	}
	browseCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, e := exec.CommandContext(browseCtx, binary, "--parsable", "--resolve", "--terminate", "_hgw-ca._tcp").Output()
	if e != nil {
		return DiscoverPortable(ctx, ports...)
	}
	found := []Service{}
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.Split(line, ";")
		if len(parts) < 9 || parts[0] != "=" || parts[4] != "_hgw-ca._tcp" {
			continue
		}
		ip := net.ParseIP(parts[7])
		if ip == nil || !permittedIP(ip) {
			continue
		}
		port, e := strconv.Atoi(parts[8])
		if e != nil || port < 1 || port > 65535 {
			continue
		}
		if len(found) >= 50 {
			break
		}
		found = append(found, Service{Name: parts[3], Host: parts[6], Address: ip.String(), Port: port, Channel: "lan-mdns-unverified"})
	}
	udp, _ := DiscoverPortable(ctx, ports...)
	seen := map[string]bool{}
	for _, peer := range found {
		seen[peer.Address] = true
	}
	for _, peer := range udp {
		if !seen[peer.Address] {
			found = append(found, peer)
		}
	}
	return found, nil
}
