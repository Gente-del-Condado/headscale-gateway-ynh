package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"

	"os"
	"path/filepath"
	"strings"

	"tailscale.com/tsnet"
)

// tailscaleIdentityFile es el nombre del archivo utilizado para
// persistir el nombre corto del nodo que Tailscale/Headscale asignó
// al Gateway.
//
// El archivo se almacena dentro de:
//
//	<dataDir>/identity/tailscale-name
//
// El contenido es únicamente el nombre corto, por ejemplo:
//
//	equipo-1
//
// No se almacena el FQDN completo.
const tailscaleIdentityFile = "tailscale-name"

// loadStoredTailscaleName carga el nombre Tailscale conocido
// anteriormente.
//
// Si todavía nunca hemos conseguido conectar con Tailscale,
// devuelve una cadena vacía.
//
// El archivo contiene únicamente el nombre corto del nodo:
//
//	equipo-1
//
// No contiene el dominio completo.
func loadStoredTailscaleName(
	dataDir string,
) (string, error) {

	// Directorio donde se almacena el estado de identidad persistente.
	identityDir := filepath.Join(
		dataDir,
		"identity",
	)

	// Ruta completa del archivo que contiene el nombre Tailscale.
	path := filepath.Join(
		identityDir,
		tailscaleIdentityFile,
	)

	// Lee el contenido completo del archivo.
	data, err := os.ReadFile(path)
	if err != nil {

		// La ausencia del archivo significa simplemente que
		// todavía no existe una identidad Tailscale persistida.
		if os.IsNotExist(err) {
			return "", nil
		}

		// Cualquier otro error de lectura sí debe propagarse.
		return "", fmt.Errorf(
			"leyendo identidad Tailscale %s: %w",
			path,
			err,
		)
	}

	// Elimina espacios y saltos de línea del contenido almacenado.
	name := strings.TrimSpace(string(data))

	// Un archivo vacío se considera equivalente a no tener
	// una identidad almacenada.
	if name == "" {
		return "", nil
	}

	// Normaliza el nombre antes de devolverlo para garantizar
	// que la identidad utilizada por el Gateway mantiene el mismo
	// formato DNS que el resto del proyecto.
	return normalizeDNSName(name), nil
}

// saveTailscaleName guarda permanentemente el nombre Tailscale
// que Headscale ha asignado al nodo.
//
// Este nombre se convierte en la identidad estable del Gateway
// una vez que Tailscale ha conectado correctamente.
func saveTailscaleName(
	dataDir string,
	name string,
) error {

	// Normaliza el nombre antes de almacenarlo.
	name = normalizeDNSName(name)

	// No permitimos persistir una identidad vacía.
	if name == "" {
		return fmt.Errorf(
			"no se puede guardar una identidad Tailscale vacía",
		)
	}

	// Directorio donde se almacenará la identidad persistente.
	identityDir := filepath.Join(
		dataDir,
		"identity",
	)

	// Crea el directorio con los permisos privados definidos
	// por mkdirPrivate().
	if err := mkdirPrivate(identityDir); err != nil {
		return fmt.Errorf(
			"creando directorio de identidad %s: %w",
			identityDir,
			err,
		)
	}

	// Ruta final del archivo de identidad.
	path := filepath.Join(
		identityDir,
		tailscaleIdentityFile,
	)

	// Guarda el nombre seguido de un salto de línea.
	//
	// Los permisos 0600 hacen que solamente el usuario propietario
	// pueda leer o modificar el archivo.
	if err := os.WriteFile(
		path,
		[]byte(name+"\n"),
		0600,
	); err != nil {
		return fmt.Errorf(
			"guardando identidad Tailscale %s: %w",
			path,
			err,
		)
	}
	diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-IDENTITY-001", "Identidad Tailscale guardada: %s", name)

	return nil
}

// tailscaleNodeName obtiene el nombre corto del nodo a partir
// del FQDN que devuelve Tailscale.
//
// Ejemplo:
//
//	equipo-1.intra.example.net
//
// se convierte en:
//
//	equipo-1
//
// Importante:
//
//	No utilizamos normalizeDNSName() sobre el FQDN completo,
//	porque esa función convierte los puntos en guiones.
//
//	Para analizar un FQDN debemos conservar los separadores '.'
//	hasta extraer el nombre del nodo.
func tailscaleNodeName(
	dnsName string,
	baseDomain string,
) (string, error) {

	// Elimina espacios accidentales alrededor del DNSName recibido.
	dnsName = strings.TrimSpace(
		dnsName,
	)

	// Tailscale puede devolver el FQDN terminado en punto.
	// Se elimina ese punto final antes de realizar las comparaciones.
	dnsName = strings.TrimSuffix(
		dnsName,
		".",
	)

	// Limpia igualmente el dominio base recibido desde la configuración.
	baseDomain = strings.TrimSpace(
		baseDomain,
	)

	baseDomain = strings.TrimSuffix(
		baseDomain,
		".",
	)

	// Las comparaciones de nombres DNS se realizan sin distinguir
	// mayúsculas y minúsculas.
	dnsName = strings.ToLower(
		dnsName,
	)

	baseDomain = strings.ToLower(
		baseDomain,
	)

	// No es posible extraer una identidad de un DNSName vacío.
	if dnsName == "" {
		return "", fmt.Errorf(
			"DNSName Tailscale vacío",
		)
	}

	// El dominio base también es necesario para poder determinar
	// qué parte del FQDN corresponde al nombre del nodo.
	if baseDomain == "" {
		return "", fmt.Errorf(
			"BaseDomain vacío",
		)
	}

	// Construye el sufijo que debe terminar el FQDN de Tailscale.
	//
	// Por ejemplo:
	//
	//	.intra.example.net
	suffix := "." + baseDomain

	// Comprueba que el nombre proporcionado pertenece realmente
	// al dominio base configurado.
	if !strings.HasSuffix(
		dnsName,
		suffix,
	) {
		return "", fmt.Errorf(
			"DNSName Tailscale %q no pertenece al dominio base %q",
			dnsName,
			baseDomain,
		)
	}

	// Elimina el dominio base para obtener únicamente el nombre
	// que identifica al nodo.
	nodeName := strings.TrimSuffix(
		dnsName,
		suffix,
	)

	// El FQDN debe contener una identidad antes del dominio base.
	if nodeName == "" {
		return "", fmt.Errorf(
			"DNSName Tailscale %q no contiene nombre de nodo",
			dnsName,
		)
	}

	// El diseño actual espera que el nombre del nodo sea una única
	// etiqueta DNS.
	//
	// Un resultado como:
	//
	//	nodo.subdominio.intra.example.net
	//
	// no se acepta como identidad de nodo.
	if strings.Contains(
		nodeName,
		".",
	) {
		return "", fmt.Errorf(
			"DNSName Tailscale inesperado %q: contiene varias etiquetas antes de BaseDomain",
			dnsName,
		)
	}

	// Una vez aislado el nombre corto, ya podemos utilizar
	// normalizeDNSName() para aplicar las reglas de normalización
	// habituales del proyecto.
	nodeName = normalizeDNSName(
		nodeName,
	)

	// Comprobación final para evitar devolver una identidad vacía.
	if nodeName == "" {
		return "", fmt.Errorf(
			"nombre de nodo Tailscale vacío después de normalizar",
		)
	}

	return nodeName, nil
}

// resolveNodeIdentity determina la identidad efectiva del Gateway.
//
// Prioridad:
//
//  1. Nombre Tailscale ya conocido y persistido.
//  2. Nombre Tailscale descubierto durante esta ejecución.
//  3. Hostname del sistema como identidad provisional.
//
// Cuando existe una identidad persistida y Headscale devuelve otra,
// se registra la discrepancia sin cambiar automáticamente el nombre.
// Esto evita modificar inesperadamente los dominios, certificados y
// rutas de los servicios publicados.
//
// Si no existe una identidad persistida y falla la consulta de
// identidad a Headscale, el Gateway puede seguir funcionando por LAN
// con el hostname del sistema. Ese nombre provisional no se persiste.
func resolveNodeIdentity(
	dataDir string,
	osHostname string,
	cfg Config,
	tsServer *tsnet.Server,
) (string, error) {

	// Recupera la identidad estable utilizada en ejecuciones anteriores.
	storedName, err := loadStoredTailscaleName(dataDir)
	if err != nil {
		return "", err
	}

	// Prepara un nombre provisional para el caso de que todavía no
	// exista una identidad conocida o no pueda consultarse Headscale.
	fallbackName := normalizeDNSName(osHostname)

	// Si hay conexión con tsnet, intentamos consultar el nombre real
	// asignado por Headscale incluso cuando ya existe uno persistido.
	if tsServer != nil {
		dnsName, identityErr := tailscaleDNSName(tsServer)
		if identityErr == nil {
			var discoveredName string
			discoveredName, identityErr = tailscaleNodeName(dnsName, cfg.BaseDomain)
			if identityErr == nil {
				if storedName != "" {
					if storedName != discoveredName {
						diagnostics.Emit("gateway", diagnostics.Warn, "HGW-TAILSCALE-IDENTITY-002", "identidad Tailscale persistida %q diferente de la asignada por Headscale %q; se conserva la identidad persistida para evitar cambios de dominio y certificados", storedName,
							discoveredName,
						)
					} else {
						diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-IDENTITY-003", "Identidad Tailscale persistida confirmada por Headscale: %s", storedName)
					}
					return storedName, nil
				}

				// Primera identidad válida: se guarda para siguientes arranques.
				if err := saveTailscaleName(dataDir, discoveredName); err != nil {
					return "", err
				}
				return discoveredName, nil
			}
		}
		diagnostics.

			// Un fallo al consultar o interpretar la identidad no debe
			// impedir que el Gateway continúe funcionando por LAN.
			Emit("gateway", diagnostics.Warn, "HGW-TAILSCALE-IDENTITY-004", "no se pudo obtener la identidad Tailscale de Headscale: %v", identityErr)
	}

	// Con una identidad persistida se mantiene la estabilidad incluso
	// si Headscale no responde o no devuelve una identidad utilizable.
	if storedName != "" {
		diagnostics.Emit("gateway", diagnostics.InfoLevel, "HGW-TAILSCALE-IDENTITY-005", "Identidad Tailscale persistida: %s", storedName)
		return storedName, nil
	}

	// No guardamos el hostname provisional: al recuperarse Headscale
	// podrá descubrirse y persistirse la identidad real del nodo.
	if fallbackName == "" {
		return "", fmt.Errorf("hostname del sistema vacío después de normalizar; no se puede determinar la identidad provisional")
	}
	diagnostics.Emit("gateway", diagnostics.Warn, "HGW-TAILSCALE-IDENTITY-006", "identidad Tailscale no disponible; usando hostname provisional para LAN: %s", fallbackName)
	return fallbackName, nil
}
