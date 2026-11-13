// El paquete portal define el directorio opcional de servicios y su catálogo público.
// Nunca contiene direcciones de backend, credenciales ni endpoints administrativos.
package portal

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/png"
	"net/url"
	"regexp"
	"strings"
)

const CatalogPath = "/.well-known/headscale-gateway"
const SettingsKey = "portal.settings.v1"

type Local struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Hidden      bool   `json:"hidden"`
	Share       bool   `json:"share"`
}
type Selection struct {
	Peer string `json:"peer"`
	ID   string `json:"id"`
}
type Config struct {
	Enabled     bool             `json:"enabled"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Accent      string           `json:"accent"`
	Theme       string           `json:"theme"`
	Logo        string           `json:"logo"`
	Favicon     string           `json:"favicon"`
	Search      bool             `json:"search"`
	ShowAdmin   bool             `json:"show_admin"`
	Publish     bool             `json:"publish"`
	Federation  bool             `json:"federation"`
	Discovery   bool             `json:"discovery"`
	Local       map[string]Local `json:"local"`
	Selected    []Selection      `json:"selected"`
}
type Entry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	URL         string `json:"url"`
	Gateway     string `json:"gateway,omitempty"`
}
type Catalog struct {
	Protocol string  `json:"protocol"`
	Version  int     `json:"version"`
	Gateway  string  `json:"gateway"`
	Origin   string  `json:"origin"`
	Services []Entry `json:"services"`
}

func Default() Config {
	return Config{Enabled: true, Title: "Servicios", Description: "Portal privado de servicios", Accent: "#38bdf8", Theme: "dark", Search: true, ShowAdmin: true, Local: map[string]Local{}, Selected: []Selection{}}
}

var color = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func Image(value string) ([]byte, error) {
	if value == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(raw) > 128*1024 {
		return nil, errors.New("imagen PNG inválida o superior a 128 KiB")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || format != "png" || cfg.Width > 1024 || cfg.Height > 1024 {
		return nil, errors.New("se requiere PNG de hasta 1024 por 1024 píxeles")
	}
	return raw, nil
}
func (c Config) Validate() error {
	if strings.TrimSpace(c.Title) == "" || len(c.Title) > 100 || len(c.Description) > 400 || !color.MatchString(c.Accent) || (c.Theme != "dark" && c.Theme != "light") {
		return errors.New("personalización del portal inválida")
	}
	if len(c.Local) > 500 || len(c.Selected) > 200 {
		return errors.New("demasiados servicios en el portal")
	}
	for name, item := range c.Local {
		if len(name) > 253 || len(item.Label) > 100 || len(item.Description) > 400 || len(item.Group) > 60 {
			return errors.New("descripción de servicio demasiado larga")
		}
	}
	for _, s := range c.Selected {
		if len(s.Peer) != 64 || len(s.ID) > 253 || s.ID == "" {
			return errors.New("selección remota inválida")
		}
	}
	if _, err := Image(c.Logo); err != nil {
		return err
	}
	_, err := Image(c.Favicon)
	return err
}

// Validate limita cada enlace anunciado a un subdominio inmediato del portal
// solicitado. Los catálogos remotos no pueden introducir URL externas.
func (c Catalog) Validate(origin string) error {
	u, e := url.Parse(origin)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" {
		return errors.New("origen de catálogo inválido")
	}
	if c.Protocol != "headscale-gateway" || c.Version != 1 || c.Origin != strings.TrimSuffix(origin, "/") || len(c.Gateway) > 100 || len(c.Services) > 500 {
		return errors.New("catálogo de Gateway inválido")
	}
	seen := map[string]bool{}
	for _, s := range c.Services {
		v, e := url.Parse(s.URL)
		if e != nil || v.Scheme != "https" || v.User != nil || v.RawQuery != "" || v.Fragment != "" || v.Path != "" && v.Path != "/" || effectivePort(v) != effectivePort(u) {
			return errors.New("enlace remoto inválido")
		}
		label := strings.TrimSuffix(v.Hostname(), "."+u.Hostname())
		if label == v.Hostname() || strings.Contains(label, ".") || label == "" || s.ID != label || seen[s.ID] || len(s.Name) > 100 || len(s.Description) > 400 || len(s.Group) > 60 {
			return errors.New("el catálogo contiene servicios ajenos al portal principal")
		}
		seen[s.ID] = true
	}
	return nil
}

func effectivePort(u *url.URL) string {
	if u.Port() == "" {
		return "443"
	}
	return u.Port()
}
