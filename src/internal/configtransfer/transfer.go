// Package configtransfer implementa perfiles de configuración transportables.
// No incluye el estado de Tailscale, claves de CA ni la base SQLite.
// Cada perfil está cifrado de extremo a extremo con una contraseña distinta
// y contiene exclusivamente opciones conocidas del Gateway.
package configtransfer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gentedelcondado/headscale-gateway/internal/securebackup"
)

const maxPlain = 256 << 10
const maxCipher = 320 << 10

type Profile struct {
	Format          string            `json:"format"`
	Version         int               `json:"version"`
	CreatedAt       time.Time         `json:"created_at"`
	IncludesSecrets bool              `json:"includes_secrets"`
	Settings        map[string]string `json:"settings"`
}

// Export produce un único archivo .hgwcfg cifrado autenticado. La contraseña
// no se envía a ningún servicio externo, pero sí al Gateway local por HTTPS.
func Export(settings map[string]string, withSecrets bool, password string) ([]byte, error) {
	if len(settings) == 0 {
		return nil, errors.New("no hay ajustes para exportar")
	}
	p := Profile{Format: "hgw-config", Version: 1, CreatedAt: time.Now().UTC(), IncludesSecrets: withSecrets, Settings: settings}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxPlain {
		return nil, errors.New("configuración demasiado grande")
	}
	key, err := securebackup.GenerateKey(password)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	writer, err := securebackup.NewWriter(&output, key)
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(raw); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Import rechaza cargas malformadas, truncadas, excesivas o de otra versión.
// Se debe leer hasta EOF para verificar la autenticidad de la última trama.
func Import(encrypted []byte, password string) (Profile, error) {
	if len(encrypted) == 0 || len(encrypted) > maxCipher {
		return Profile{}, errors.New("archivo de configuración vacío o demasiado grande")
	}
	reader, err := securebackup.NewReader(bytes.NewReader(encrypted), password)
	if err != nil {
		return Profile{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxPlain+1))
	if err != nil {
		return Profile{}, fmt.Errorf("no se pudo verificar el archivo cifrado: %w", err)
	}
	if len(raw) > maxPlain {
		return Profile{}, errors.New("configuración demasiado grande")
	}
	var result Profile
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&result); err != nil {
		return Profile{}, errors.New("contenido de configuración inválido")
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return Profile{}, errors.New("contenido adicional no permitido")
	}
	if result.Format != "hgw-config" || result.Version != 1 || len(result.Settings) == 0 {
		return Profile{}, errors.New("versión o formato de configuración no admitidos")
	}
	return result, nil
}
