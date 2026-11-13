package pki

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"
)

// Descubrimiento LAN portátil, adicional al DNS-SD de Avahi. Los anuncios son
// SOLO PISTAS: un tercero en LAN puede falsificarlos. No se confía en ninguna
// CA, no se descargan claves y no se emiten certificados por descubrir peers.
const discoveryGroup = "239.255.42.91:47991"
const discoveryMagic = "HGW-PEER-DISCOVERY/1"

func resolveDiscoveryGroup(ports ...int) (*net.UDPAddr, error) {
	port := 47991
	if len(ports) > 0 && ports[0] != 0 {
		port = ports[0]
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("puerto de descubrimiento fuera del intervalo 1..65535")
	}
	return net.ResolveUDPAddr("udp4", net.JoinHostPort("239.255.42.91", strconv.Itoa(port)))
}

type discoveryFrame struct {
	Magic    string `json:"magic"`
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	CAPort   int    `json:"ca_http_port,omitempty"`
}

// Service describe un candidato no autenticado, nunca una relación de confianza.
type Service struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Endpoint string `json:"endpoint,omitempty"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	Channel  string `json:"channel"`
	CAPort   int    `json:"ca_http_port,omitempty"`
}

func discoveryReply(data []byte, source net.IP) (Service, bool) {
	if len(data) == 0 || len(data) > 2048 || !permittedIP(source) {
		return Service{}, false
	}
	var msg discoveryFrame
	if json.Unmarshal(data, &msg) != nil || msg.Magic != discoveryMagic || msg.Type != "peer" || msg.CAPort < 0 || msg.CAPort > 65535 || len(msg.Name) > 120 || len(msg.Endpoint) > 512 {
		return Service{}, false
	}
	url, err := PeerURL(msg.Endpoint)
	if err != nil {
		return Service{}, false
	}
	port := 443
	if url.Port() != "" {
		port, _ = strconv.Atoi(url.Port())
	}
	return Service{Name: msg.Name, Host: url.Hostname(), Endpoint: url.String(), Address: source.String(), Port: port, CAPort: msg.CAPort, Channel: "lan-udp-unverified"}, true
}

// DiscoverUDP solicita respuestas multicast y espera hasta dos segundos.
// Si la red bloquea multicast, el usuario puede añadir el peer manualmente.
func DiscoverUDP(ctx context.Context, ports ...int) ([]Service, error) {
	group, err := resolveDiscoveryGroup(ports...)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	probe, _ := json.Marshal(discoveryFrame{Magic: discoveryMagic, Type: "probe"})
	if _, err = conn.WriteToUDP(probe, group); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	found := make([]Service, 0)
	seen := map[string]bool{}
	buf := make([]byte, 2049)
	for len(found) < 50 {
		_ = conn.SetReadDeadline(deadline)
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			return found, err
		}
		if candidate, ok := discoveryReply(buf[:n], src.IP); ok && !seen[candidate.Address] {
			found = append(found, candidate)
			seen[candidate.Address] = true
		}
	}
	return found, nil
}

// ServeDiscovery anuncia solo Gateways con publicación de CA activada y un
// subdominio HTTPS alcanzable por LAN. El ciclo termina con el contexto.
// El sistema operativo puede requerir autorizar el UDP entrante 47991.
func ServeDiscovery(ctx context.Context, endpoint func() string, caPort func() int, port int) error {
	group, err := resolveDiscoveryGroup(port)
	if err != nil {
		return err
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(8192)
	buf := make([]byte, 2049)
	for ctx.Err() == nil {
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		if n > 2048 || src == nil || !permittedIP(src.IP) {
			continue
		}
		var q discoveryFrame
		if json.Unmarshal(buf[:n], &q) != nil || q.Magic != discoveryMagic || q.Type != "probe" {
			continue
		}
		origin := strings.TrimSpace(endpoint())
		if _, err = PeerURL(origin); err != nil {
			continue
		}
		u, _ := PeerURL(origin)
		httpPort := 0
		if caPort != nil {
			httpPort = caPort()
		}
		reply, _ := json.Marshal(discoveryFrame{CAPort: httpPort, Magic: discoveryMagic, Type: "peer", Name: u.Hostname(), Endpoint: origin})
		// El puerto UDP de respuesta del cliente puede ser efímero.
		_, _ = conn.WriteToUDP(reply, src)
	}
	return ctx.Err()
}

// DiscoverPortable mantiene un único contrato para Linux, Windows y macOS.
func DiscoverPortable(ctx context.Context, ports ...int) ([]Service, error) {
	peers, err := DiscoverUDP(ctx, ports...)
	if err != nil {
		return nil, errors.New("no se pudo emitir búsqueda UDP en LAN; revisa multidifusión y firewall")
	}
	return peers, nil
}
