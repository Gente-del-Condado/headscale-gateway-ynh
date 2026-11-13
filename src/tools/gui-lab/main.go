// gui-lab es un laboratorio loopback aislado para pruebas manuales del cliente de escritorio.
// No contacta con Headscale, cambia la confianza del sistema ni carga datos productivos.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:19091")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var mu sync.RWMutex
	pending, available := false, true
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/admin/" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "Laboratorio GUI: datos ficticios. Usa r en la consola para cambiar las rutas pendientes. Este no es el panel real del Gateway.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			fmt.Fprint(w, `{"error":"read only"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer hgw-lab-only" {
			w.WriteHeader(401)
			fmt.Fprint(w, `{"error":"laboratory token required"}`)
			return
		}
		mu.RLock()
		p, a := pending, available
		mu.RUnlock()
		if !a {
			w.WriteHeader(503)
			fmt.Fprint(w, `{"error":"simulated outage"}`)
			return
		}
		switch r.URL.Path {
		case "/api/v1/network/info":
			json.NewEncoder(w).Encode(map[string]any{"gateway_vpn_state": "Laboratorio (datos ficticios)"})
		case "/api/v1/network/nodes":
			routes := []string{}
			if p {
				routes = []string{"192.0.2.0/24"}
			}
			json.NewEncoder(w).Encode(map[string]any{"nodes": []any{map[string]any{"id": "lab-1", "name": "win11-laboratorio", "given_name": "Windows de prueba", "user": "laboratorio", "online": true, "ip_addresses": []string{"192.0.2.10"}, "available_routes": routes, "approved_routes": []string{}}}})
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"fixture endpoint not found"}`)
		}
	})
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	fmt.Println("LABORATORIO GUI - sin datos ni servicios reales")
	fmt.Println("URL: http://127.0.0.1:19091")
	fmt.Println("Token de prueba PUBLICO, solo valido aqui: hgw-lab-only")
	fmt.Println("r + Enter: alternar solicitud de ruta; c + Enter: alternar fallo API; q + Enter: salir")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if command == "q" {
			break
		}
		mu.Lock()
		switch command {
		case "r":
			pending = !pending
			fmt.Println("Ruta pendiente:", pending)
		case "c":
			available = !available
			fmt.Println("API disponible:", available)
		default:
			fmt.Println("Usa r, c o q y Enter")
		}
		mu.Unlock()
	}
	server.Close()
}
