package main

// Circuit breaker ACME. Un HTTP 429 no se cura reiniciando el proceso:
// la próxima fecha de reintento se persiste cifrada en la SQLite privada.
// El identificador incluye dominio, cuenta y proveedor para no bloquear
// configuraciones independientes.
import (
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

	"github.com/gentedelcondado/headscale-gateway/internal/state"

	"regexp"
	"strings"
	"time"
)

// El mensaje que emite ACME RFC 8555 incluye, además de Retry-After,
// una fecha explícita emitida por Let's Encrypt.
var acmeRetryPattern = regexp.MustCompile(`(?i)retry after (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) UTC`)

type acmeCooldown struct {
	Until    time.Time `json:"until"`
	Category string    `json:"category"`
}

func acmeRetryTime(err error, now time.Time) acmeCooldown {
	if err == nil {
		return acmeCooldown{}
	}
	message := err.Error()
	match := acmeRetryPattern.FindStringSubmatch(message)
	if len(match) > 1 {
		if until, parseErr := time.ParseInLocation("2006-01-02 15:04:05", match[1], time.UTC); parseErr == nil {
			until = until.Add(2 * time.Minute) // margen frente a desfase reloj y propagación
			if until.After(now) {
				return acmeCooldown{Until: until, Category: "rate_limit"}
			}
		}
	}
	if strings.Contains(message, "HTTP 429") || strings.Contains(message, "rateLimited") {
		return acmeCooldown{Until: now.Add(60 * time.Minute), Category: "rate_limit"}
	}
	return acmeCooldown{Until: now.Add(30 * time.Minute), Category: "temporary_error"}
}

func acmeCooldownKey(cfg Config, domain string) string {
	return fmt.Sprintf("acme.cooldown.%s.%s.%s", cfg.ACMEDNSProvider, cfg.ACMEEmail, domain)
}

func readACMEBackoff(cfg Config, domain string) time.Time {
	if cfg.DatabasePath == "" {
		return time.Time{}
	}
	db, err := state.Open(cfg.DatabasePath)
	if err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-ACME-STATE", "No se pudo consultar el estado ACME en SQLite; se aplaza el intento un minuto: %v", err)
		return time.Now().Add(time.Minute)
	}
	defer db.Close()
	var cooldown acmeCooldown
	found, err := db.GetJSON(acmeCooldownKey(cfg, domain), &cooldown)
	if err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-ACME-STATE", "Estado ACME ilegible; se aplaza el intento un minuto: %v", err)
		return time.Now().Add(time.Minute)
	}
	if !found {
		return time.Time{}
	}
	return cooldown.Until
}

func storeACMEBackoff(cfg Config, domain string, failure error) {
	if cfg.DatabasePath == "" {
		return
	}
	db, err := openCatalog(cfg.DatabasePath)
	if err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-ACME-BACKOFF-001", "no se puede guardar backoff ACME: %v", err)
		return
	}
	defer db.Close()
	next := acmeRetryTime(failure, time.Now().UTC())
	if err := db.PutJSON(acmeCooldownKey(cfg, domain), next); err != nil {
		diagnostics.Emit("gateway", diagnostics.Warn, "HGW-ACME-BACKOFF-002", "persistencia backoff ACME fallida: %v", err)
	} else if failure != nil {
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-ACME-BACKOFF-003", "próximo intento para %s a partir de %s (%s)", domain, next.Until.Format(time.RFC3339), next.Category)
	}
}

// El certificado existente sólo se reutiliza si están disponibles ambos
// nombres: dominio del nodo y wildcard para los subdominios de servicios.
// Evita ofrecer una generación en la que sólo el portal principal tiene TLS.
func usableCachedACME(load func(string) error, nodeDomain, wildcardDomain string) error {
	if err := load(nodeDomain); err != nil {
		return fmt.Errorf("dominio principal: %w", err)
	}
	if err := load(wildcardDomain); err != nil {
		return fmt.Errorf("wildcard: %w", err)
	}
	return nil
}
