//go:build android

package main

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"tailscale.com/net/netmon"
	"tailscale.com/net/socks5"
	"tailscale.com/tsnet"
)

// Android proporciona las interfaces a Go: el sandbox restringe netlink.
// Patrón contrastado con TailSocks/appctr/netmon_android.go (BSD-3-Clause).
type mobileNetwork struct {
	Default    string `json:"default"`
	Interfaces []struct {
		Name      string   `json:"name"`
		Index     int      `json:"index"`
		MTU       int      `json:"mtu"`
		Addresses []string `json:"addresses"`
	} `json:"interfaces"`
}

var mobileNetworkMu sync.Mutex

func mobileInterfaces() ([]netmon.Interface, error) {
	mobileNetworkMu.Lock()
	defer mobileNetworkMu.Unlock()
	var snapshot mobileNetwork
	if err := readPrivateJSON(os.Getenv("GATEWAY_ANDROID_NETWORK_FILE"), &snapshot); err != nil {
		return nil, err
	}
	netmon.UpdateLastKnownDefaultRouteInterface(snapshot.Default)
	result := make([]netmon.Interface, 0, len(snapshot.Interfaces))
	for _, item := range snapshot.Interfaces {
		iface := netmon.Interface{Interface: &net.Interface{Name: item.Name, Index: item.Index, MTU: item.MTU, Flags: net.FlagUp | net.FlagRunning}}
		if item.Name == "lo" {
			iface.Flags |= net.FlagLoopback
		}
		for _, address := range item.Addresses {
			ip, prefix, err := net.ParseCIDR(address)
			if err == nil {
				prefix.IP = ip
				iface.AltAddrs = append(iface.AltAddrs, prefix)
			}
		}
		result = append(result, iface)
	}
	return result, nil
}

func init() {
	// El motor no puede quedar huérfano si Android mata el proceso del servicio.
	if os.Getenv("GATEWAY_ANDROID_NETWORK_FILE") != "" {
		runtime.LockOSThread()
		parent := os.Getppid()
		if err := unix.Prctl(unix.PR_SET_PDEATHSIG, uintptr(syscall.SIGTERM), 0, 0, 0); err != nil || parent == 1 || os.Getppid() != parent {
			os.Exit(1)
		}
	}
	if os.Getenv("GATEWAY_ANDROID_NETWORK_FILE") != "" {
		netmon.RegisterInterfaceGetter(mobileInterfaces)
	}
}

func startMobileServices(runtime *gatewayRuntime, vpn *vpnState) error {
	secret := os.Getenv("GATEWAY_ANDROID_SECRET")
	if len(secret) != 64 {
		return errors.New("falta la credencial privada del adaptador Android")
	}
	listen := func(key string) (net.Listener, error) {
		port, err := strconv.Atoi(os.Getenv(key))
		if err != nil || port < 1024 || port > 65535 {
			return nil, fmt.Errorf("puerto Android inválido: %s", key)
		}
		return net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	}
	control, err := listen("GATEWAY_ANDROID_CONTROL_PORT")
	if err != nil {
		return err
	}
	proxy, err := listen("GATEWAY_ANDROID_SOCKS_PORT")
	if err != nil {
		control.Close()
		return err
	}
	socks := &socks5.Server{Username: "gateway", Password: secret, Dialer: func(ctx context.Context, network, address string) (net.Conn, error) {
		vpn.mu.Lock()
		server := vpn.server
		vpn.mu.Unlock()
		if server == nil {
			return nil, errors.New("la conexión VPN del Gateway no está lista")
		}
		// Dial a quad-100 no equivale a consultar el resolvedor interno en
		// tsnet. Se conserva el gestor DNS del nodo, sus rutas y políticas.
		if address == "100.100.100.100:53" || address == "[fd7a:115c:a1e0::53]:53" {
			return mobileDNSConn(ctx, server, network), nil
		}
		return server.Dial(ctx, network, address)
	}}
	go socks.Serve(proxy)
	bootID := time.Now().UTC().Format(time.RFC3339Nano)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		vpn.mu.Lock()
		server := vpn.server
		vpn.mu.Unlock()
		value := map[string]any{"connected": false, "services": len(runtime.currentConfig().Services), "boot_id": bootID}
		if server != nil {
			client, err := server.LocalClient()
			if err == nil {
				status, err := client.Status(r.Context())
				if err == nil {
					value["connected"] = status.BackendState == "Running"
					value["ips"] = status.TailscaleIPs
				}
				if prefs, err := client.GetPrefs(r.Context()); err == nil {
					value["lan_bypass"] = (prefs.ExitNodeID.IsZero() && !prefs.ExitNodeIP.IsValid()) || prefs.ExitNodeAllowLANAccess
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(value)
	})
	mux.HandleFunc("POST /network", func(w http.ResponseWriter, r *http.Request) {
		_, err := mobileInterfaces()
		if err != nil {
			http.Error(w, "No se puede leer la red", 400)
			return
		}
		vpn.mu.Lock()
		server := vpn.server
		vpn.mu.Unlock()
		if server != nil {
			server.Sys().NetMon.Get().Poll()
		}
		w.WriteHeader(204)
	})
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+secret)) != 1 {
			http.Error(w, "Acceso denegado", 403)
			return
		}
		mux.ServeHTTP(w, r)
	})}
	go server.Serve(control)
	shutdownTasks.Add(1)
	go func() { defer shutdownTasks.Done(); <-processContext.Done(); proxy.Close(); shutdownHTTPServer(server) }()
	return nil
}

func mobileDNSConn(ctx context.Context, server *tsnet.Server, network string) net.Conn {
	client, bridge := net.Pipe()
	go func() {
		defer bridge.Close()
		stop := context.AfterFunc(ctx, func() { bridge.Close() })
		defer stop()
		ip4, ip6 := server.TailscaleIPs()
		from := ip4
		if !from.IsValid() {
			from = ip6
		}
		buffer := make([]byte, 65535)
		for {
			bridge.SetReadDeadline(time.Now().Add(2 * time.Minute))
			var n int
			var err error
			if network == "tcp" {
				var header [2]byte
				if _, err = io.ReadFull(bridge, header[:]); err != nil {
					return
				}
				n, err = io.ReadFull(bridge, buffer[:int(binary.BigEndian.Uint16(header[:]))])
			} else {
				// net.Pipe conserva cada escritura SOCKS UDP mientras el
				// receptor tenga espacio para el datagrama completo.
				n, err = bridge.Read(buffer)
			}
			if err != nil || n == 0 {
				return
			}
			query, cancel := context.WithTimeout(ctx, 10*time.Second)
			response, err := server.Sys().DNSManager.Get().Query(query, buffer[:n], network, netip.AddrPortFrom(from, 0))
			cancel()
			if err != nil || len(response) > 65535 {
				return
			}
			if network == "tcp" {
				response = append([]byte{byte(len(response) >> 8), byte(len(response))}, response...)
			}
			bridge.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := bridge.Write(response); err != nil {
				return
			}
		}
	}()
	return client
}
