package main

import "crypto/tls"

// enableHTTPALPN anuncia los protocolos HTTP aceptados por el gateway.
// Se invoca exclusivamente sobre la copia de tls.Config de una nueva
// generación, antes de publicarla. Conserva protocolos adicionales (por
// ejemplo, acme-tls/1) y evita duplicados.
func enableHTTPALPN(cfg *tls.Config) {
	for _, protocol := range []string{"h2", "http/1.1"} {
		found := false
		for _, configured := range cfg.NextProtos {
			if configured == protocol {
				found = true
				break
			}
		}
		if !found {
			cfg.NextProtos = append(cfg.NextProtos, protocol)
		}
	}
}
