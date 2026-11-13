package main

import "github.com/gentedelcondado/headscale-gateway/internal/diagnostics"

import (
	"fmt"

	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// serviceNameRegexp define los caracteres que no están permitidos
// dentro del nombre normalizado de un servicio.
//
// Los caracteres que no sean:
//
//	a-z
//	0-9
//	-
//
// serán eliminados por normalizeServiceName().
var serviceNameRegexp = regexp.MustCompile(
	`[^a-z0-9-]+`,
)

// dnsNameRegexp define los caracteres que no están permitidos
// dentro del nombre DNS normalizado.
//
// Los caracteres que no sean:
//
//	a-z
//	0-9
//	-
//
// serán sustituidos por guiones por normalizeDNSName().
var dnsNameRegexp = regexp.MustCompile(
	`[^a-z0-9-]+`,
)

// repeatedDashRegexp detecta uno o varios guiones consecutivos.
//
// Se utiliza para convertir secuencias como:
//
//	home---assistant
//
// en:
//
//	home-assistant
var repeatedDashRegexp = regexp.MustCompile(
	`-+`,
)

// normalizeHost normaliza un valor procedente de HTTP Host.
//
// Su objetivo es preparar el hostname recibido en una petición HTTP
// para poder compararlo de forma fiable con las entradas de proxyMap
// y con nodeDomain.
//
// La función:
//   - elimina espacios exteriores,
//   - elimina el puerto cuando Host contiene uno,
//   - elimina el punto final de un FQDN,
//   - convierte el resultado a minúsculas.
//
// Importante:
//
//	Esta función NO convierte puntos en guiones.
//
// Por tanto:
//
//	home-assistant.equipo-1.intra.example.net
//
// permanece como:
//
//	home-assistant.equipo-1.intra.example.net
func normalizeHost(host string) string {
	// Conserva el valor original únicamente para poder registrar
	// en el log si se ha producido alguna modificación.
	original := host

	// Elimina espacios al principio y al final.
	host = strings.TrimSpace(host)

	// Intenta separar hostname y puerto.
	//
	// Por ejemplo:
	//
	//	equipo-1.intra.example.net:443
	//
	// se convierte en:
	//
	//	equipo-1.intra.example.net
	//
	// Si no existe un puerto válido, se mantiene el valor original.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// An IPv6 URL without an explicit port still carries brackets in Host.
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		if ip := net.ParseIP(host[1 : len(host)-1]); ip != nil {
			host = ip.String()
		}
	}

	// Elimina un punto final de un FQDN.
	host = strings.TrimSuffix(host, ".")

	// Los nombres DNS no distinguen mayúsculas de minúsculas.
	host = strings.ToLower(host)

	// Registra la transformación únicamente cuando el valor
	// realmente ha cambiado.
	if original != host {
		diagnostics.Emit("gateway", diagnostics.Debug, "HGW-UTILS-001", "normalizeHost: %q -> %q", original,
			host,
		)
	}

	return host
}

// normalizeDNSName normaliza un nombre que se utilizará como
// identificador DNS corto o como parte de nombres internos.
//
// A diferencia de normalizeHost(), esta función SÍ convierte
// los puntos en guiones.
//
// La función:
//   - elimina espacios exteriores,
//   - elimina un punto final,
//   - convierte a minúsculas,
//   - convierte puntos en guiones,
//   - sustituye caracteres no válidos por guiones,
//   - elimina guiones consecutivos,
//   - elimina guiones iniciales y finales.
//
// Ejemplo:
//
//	hom.pc1
//
// se convierte en:
//
//	hom-pc1
//
// Importante:
//
//	Esta función está pensada para nombres cortos o identificadores,
//	no para analizar FQDN completos.
//
// Por eso tailscaleNodeName() debe separar primero el nombre del
// nodo del BaseDomain antes de utilizar normalizeDNSName().
func normalizeDNSName(value string) string {
	// Conserva el valor original para poder registrar cualquier
	// transformación realizada.
	original := value

	// Elimina espacios exteriores.
	value = strings.TrimSpace(value)

	// Elimina un posible punto final.
	value = strings.TrimSuffix(value, ".")

	// Normaliza a minúsculas.
	value = strings.ToLower(value)

	// Convertir puntos del hostname en guiones.
	//
	// Ejemplo:
	//
	//	hom.pc1 -> hom-pc1
	value = strings.ReplaceAll(value, ".", "-")

	// Cualquier carácter no válido para una etiqueta DNS
	// se convierte en un guion.
	value = dnsNameRegexp.ReplaceAllString(value, "-")

	// Evitar guiones consecutivos.
	//
	// Por ejemplo:
	//
	//	home---assistant
	//
	// pasa a:
	//
	//	home-assistant
	value = repeatedDashRegexp.ReplaceAllString(value, "-")

	// Eliminar guiones al principio y al final.
	value = strings.Trim(value, "-")

	// Registra la transformación únicamente si el resultado
	// es diferente del valor recibido originalmente.
	if original != value {
		diagnostics.Emit("gateway", diagnostics.Debug, "HGW-UTILS-002", "normalizeDNSName: %q -> %q", original,
			value,
		)
	}

	return value
}

// normalizeServiceName normaliza el nombre lógico de un servicio
// para convertirlo en una etiqueta apropiada para utilizar como
// subdominio.
//
// La función:
//   - elimina espacios exteriores,
//   - convierte a minúsculas,
//   - convierte "_" en "-",
//   - elimina los caracteres restantes que no sean a-z, 0-9 o "-",
//   - elimina guiones iniciales y finales.
//
// Por ejemplo:
//
//	HOME_ASSISTANT
//
// se convierte en:
//
//	home-assistant
//
// A diferencia de normalizeDNSName(), los puntos no se convierten
// explícitamente en guiones: cualquier carácter no permitido por
// serviceNameRegexp simplemente se elimina.
func normalizeServiceName(value string) string {
	// Conserva el valor original para registrar posibles
	// transformaciones.
	original := value

	// Elimina espacios exteriores.
	value = strings.TrimSpace(value)

	// Convierte el nombre a minúsculas.
	value = strings.ToLower(value)

	// Convierte los guiones bajos de los nombres de servicio
	// definidos en SQLite a guiones DNS.
	//
	// Ejemplo:
	//
	//	HOME_ASSISTANT -> home-assistant
	value = strings.ReplaceAll(
		value,
		"_",
		"-",
	)

	// Elimina cualquier otro carácter que no esté permitido
	// por serviceNameRegexp.
	value = serviceNameRegexp.ReplaceAllString(
		value,
		"",
	)

	// Evita que el nombre resultante comience o termine en guion.
	value = strings.Trim(
		value,
		"-",
	)

	// Registra la transformación cuando el resultado es diferente.
	if original != value {
		diagnostics.Emit("gateway", diagnostics.Debug, "HGW-UTILS-003", "normalizeServiceName: %q -> %q", original,
			value,
		)
	}

	return value
}

// getDataDir obtiene y valida el directorio principal de datos
// utilizado por el Gateway.
//
// La función comprueba que:
//   - data_dir no esté vacío.
//   - data_dir sea una ruta absoluta.
//
// No crea el directorio. La creación de los directorios necesarios
// se realiza posteriormente mediante mkdirPrivate().
func getDataDir(cfg Config) (string, error) {
	// Elimina espacios exteriores de la configuración.
	value := strings.TrimSpace(cfg.DataDir)

	// data_dir es obligatorio en este punto.
	if value == "" {
		return "", fmt.Errorf(
			"data_dir no definido",
		)
	}

	// El proyecto requiere una ruta absoluta para evitar que
	// el almacenamiento dependa del directorio de trabajo actual.
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf(
			"data_dir debe ser una ruta absoluta: %s",
			value,
		)
	}
	diagnostics.

		// Registra la ruta finalmente utilizada.
		Emit("gateway", diagnostics.InfoLevel, "HGW-UTILS-004", "data_dir configurado: %s", value)

	return value, nil
}

// mkdirPrivate comprueba que una ruta exista como directorio
// y, si no existe, la crea con permisos privados.
//
// Los directorios creados utilizan permisos:
//
//	0700
//
// Esto significa que únicamente el usuario propietario dispone
// de permisos sobre ellos.
//
// Si el directorio ya existe, la función no modifica sus permisos
// ni su contenido; únicamente comprueba que realmente sea
// un directorio.
func mkdirPrivate(path string) error {
	// Comprueba si la ruta ya existe.
	info, err := os.Stat(path)

	if err == nil {

		// La ruta existe, pero debe ser un directorio.
		if !info.IsDir() {
			return fmt.Errorf(
				"%s existe pero no es un directorio",
				path,
			)
		}
		diagnostics.

			// El directorio ya existe y puede utilizarse.
			Emit("gateway", diagnostics.Debug, "HGW-UTILS-005", "Directorio existente: %s", path)

		return nil
	}

	// Si el error no corresponde a que la ruta no exista,
	// se considera un error real al comprobar el directorio.
	if !os.IsNotExist(err) {
		return fmt.Errorf(
			"comprobando directorio %s: %w",
			path,
			err,
		)
	}

	// La ruta no existe, por lo que se crea junto con cualquier
	// directorio padre necesario.
	//
	// 0700 mantiene el árbol de almacenamiento privado para
	// el usuario que ejecuta el Gateway.
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf(
			"creando directorio %s: %w",
			path,
			err,
		)
	}
	diagnostics.

		// Registra la creación y los permisos utilizados.
		Emit("gateway", diagnostics.InfoLevel, "HGW-UTILS-006", "Directorio creado: %s (0700)", path)

	return nil
}
