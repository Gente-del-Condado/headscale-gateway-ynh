// Package network adapta la API REST de Headscale a un inventario estable
// que pueden consumir el panel web y los futuros clientes de escritorio.
package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Node struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	GivenName       string   `json:"given_name,omitempty"`
	User            string   `json:"user,omitempty"`
	IPAddresses     []string `json:"ip_addresses"`
	Online          bool     `json:"online"`
	LastSeen        string   `json:"last_seen,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	Expiry          string   `json:"expiry,omitempty"`
	RegisterMethod  string   `json:"register_method,omitempty"`
	Tags            []string `json:"tags"`
	ApprovedRoutes  []string `json:"approved_routes"`
	AvailableRoutes []string `json:"available_routes"`
	SubnetRoutes    []string `json:"subnet_routes"`
	// Datos locales de tsnet: únicamente visibles desde el nodo gateway.
	Peer *Peer `json:"peer,omitempty"`
}
type Peer struct {
	Hostname        string   `json:"hostname,omitempty"`
	OS              string   `json:"os,omitempty"`
	DNSName         string   `json:"dns_name,omitempty"`
	Online          bool     `json:"online"`
	Active          bool     `json:"active"`
	Relay           string   `json:"derp,omitempty"`
	PeerRelay       string   `json:"peer_relay,omitempty"`
	CurrentEndpoint string   `json:"current_endpoint,omitempty"`
	Endpoints       []string `json:"endpoints,omitempty"`
	RxBytes         int64    `json:"rx_bytes"`
	TxBytes         int64    `json:"tx_bytes"`
	LastHandshake   string   `json:"last_handshake,omitempty"`
	LastWrite       string   `json:"last_write,omitempty"`
	LastSeen        string   `json:"last_seen,omitempty"`
	ExitNodeOption  bool     `json:"exit_node_option"`
}
type Inventory struct {
	Nodes   []Node `json:"nodes"`
	Online  int    `json:"online"`
	Offline int    `json:"offline"`
	Source  string `json:"source"`
}

func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") {
		return errors.New("URL de Headscale inválida: usar https://servidor sin ruta, usuario ni parámetros")
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return errors.New("Headscale debe usar HTTPS (HTTP solo para localhost o IP local)")
		}
	}
	return nil
}
func client() *http.Client {
	return &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func List(ctx context.Context, baseURL, apiKey string) (Inventory, error) {
	result := Inventory{Nodes: []Node{}, Source: "headscale-api-v1"}
	if err := ValidateURL(baseURL); err != nil {
		return result, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return result, errors.New("configura headscale.api_key en el panel y reinicia el Gateway")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/node", nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client().Do(req)
	if err != nil {
		return result, fmt.Errorf("conexión con Headscale: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("Headscale responde HTTP %d (comprueba URL y API key)", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil {
		return result, err
	}
	if len(data) > 4<<20 {
		return result, errors.New("respuesta de Headscale demasiado grande")
	}
	var body struct {
		Nodes []json.RawMessage `json:"nodes"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return result, fmt.Errorf("JSON de Headscale inválido: %w", err)
	}
	for _, raw := range body.Nodes {
		var v struct {
			ID              json.RawMessage `json:"id"`
			Name            string          `json:"name"`
			GivenName       string          `json:"givenName"`
			User            json.RawMessage `json:"user"`
			IPAddresses     []string        `json:"ipAddresses"`
			Online          bool            `json:"online"`
			LastSeen        string          `json:"lastSeen"`
			CreatedAt       string          `json:"createdAt"`
			Expiry          string          `json:"expiry"`
			RegisterMethod  string          `json:"registerMethod"`
			Tags            []string        `json:"tags"`
			ApprovedRoutes  []string        `json:"approvedRoutes"`
			AvailableRoutes []string        `json:"availableRoutes"`
			SubnetRoutes    []string        `json:"subnetRoutes"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return result, fmt.Errorf("nodo mal formado: %w", err)
		}
		n := Node{ID: stringID(v.ID), Name: v.Name, GivenName: v.GivenName, User: userLabel(v.User), IPAddresses: nonNil(v.IPAddresses), Online: v.Online, LastSeen: v.LastSeen, CreatedAt: v.CreatedAt, Expiry: v.Expiry, RegisterMethod: v.RegisterMethod, Tags: nonNil(v.Tags), ApprovedRoutes: nonNil(v.ApprovedRoutes), AvailableRoutes: nonNil(v.AvailableRoutes), SubnetRoutes: nonNil(v.SubnetRoutes)}
		result.Nodes = append(result.Nodes, n)
		if n.Online {
			result.Online++
		} else {
			result.Offline++
		}
	}
	Sort(result.Nodes)
	return result, nil
}
func nonNil(x []string) []string {
	if x == nil {
		return []string{}
	}
	return x
}
func stringID(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}
func userLabel(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var v struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Email       string `json:"email"`
	}
	if json.Unmarshal(raw, &v) == nil {
		if v.DisplayName != "" {
			return v.DisplayName
		}
		if v.Name != "" {
			return v.Name
		}
		return v.Email
	}
	return ""
}
func Sort(nodes []Node) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].Online != nodes[j].Online {
			return nodes[i].Online
		}
		a, b := nodes[i].GivenName, nodes[j].GivenName
		if a == "" {
			a = nodes[i].Name
		}
		if b == "" {
			b = nodes[j].Name
		}
		return strings.ToLower(a) < strings.ToLower(b)
	})
}
