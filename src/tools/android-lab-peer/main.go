// Nodo de laboratorio; no se incorpora al motor ni a la APK.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"tailscale.com/tsnet"
)

func main() {
	data := flag.String("data", "", "Directorio temporal privado")
	keyFile := flag.String("key-file", "", "Clave de alta del laboratorio")
	gateway := flag.String("gateway", "", "IP VPN del Gateway")
	caFile := flag.String("ca-file", "", "CA pública del Gateway para verificar su HTTPS")
	controlURL := flag.String("control-url", "http://127.0.0.1:18480", "URL del Headscale temporal")
	controlCA := flag.String("control-ca-file", "", "CA del Headscale temporal")
	serviceHost := flag.String("service-host", "prueba.android-gateway.lab.example.test", "Nombre HTTPS publicado por el Gateway")
	done := flag.Bool("done", false, "Terminar la comprobación del servicio")
	serve := flag.Bool("serve", false, "Mantener un servidor remoto y el cliente de comprobación")
	flag.Parse()
	if *controlCA != "" {
		os.Setenv("SSL_CERT_FILE", *controlCA)
	}
	roots := x509.NewCertPool()
	if *gateway != "" {
		pem, err := os.ReadFile(*caFile)
		if err != nil || !roots.AppendCertsFromPEM(pem) {
			panic("se requiere una CA pública válida para verificar el servicio HTTPS")
		}
	}
	key, err := os.ReadFile(*keyFile)
	if err != nil {
		panic(err)
	}
	s := &tsnet.Server{Dir: *data, Hostname: "laboratorio", ControlURL: *controlURL, AuthKey: strings.TrimSpace(string(key)), UserLogf: func(string, ...any) {}, Logf: func(string, ...any) {}}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err := s.Up(ctx); err != nil {
		panic(err)
	}
	fetch := func(complete bool) error {
		transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			return s.Dial(ctx, network, net.JoinHostPort(*gateway, "443"))
		},
			TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
		client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
		path := "/"
		if complete {
			path = "/done"
		}
		resp, err := client.Get("https://" + *serviceHost + path)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil || resp.StatusCode != 200 || !strings.Contains(string(body), "Servicio Android") {
			return fmt.Errorf("servicio: %d", resp.StatusCode)
		}
		return nil
	}
	if *gateway != "" && !*serve {
		if err := fetch(*done); err != nil {
			panic(err)
		}
		fmt.Println("Servicio HTTPS Android accesible desde otro nodo")
		return
	}
	ln, err := s.Listen("tcp", ":18082")
	if err != nil {
		panic(err)
	}
	ip4, _ := s.TailscaleIPs()
	fmt.Println(ip4)
	udp, err := s.ListenPacket("udp", net.JoinHostPort(ip4.String(), "18083"))
	if err != nil {
		panic(err)
	}
	defer udp.Close()
	go func() {
		buffer := make([]byte, 2048)
		for {
			n, address, err := udp.ReadFrom(buffer)
			if err != nil {
				return
			}
			udp.WriteTo(buffer[:n], address)
		}
	}()
	if *serve {
		control, err := net.Listen("tcp", "127.0.0.1:18482")
		if err != nil {
			panic(err)
		}
		go http.Serve(control, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := fetch(r.URL.Path == "/done"); err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			fmt.Fprint(w, "Servicio HTTPS Android comprobado")
		}))
	}
	if err := http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "Nodo de laboratorio") })); err != nil {
		panic(err)
	}
}
