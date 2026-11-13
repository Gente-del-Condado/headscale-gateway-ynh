// Package forgejostore transporta archivos cifrados como adjuntos de Forgejo.
package forgejostore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	Profile    = "configuracion.hgwcfg"
	Backup     = "copia.hgw"
	MaxProfile = 320 << 10
	MaxBackup  = 2<<30 - 1
	marker     = "headscale-gateway-sha256: "
)

var segment = regexp.MustCompile(`^[A-Za-z0-9_.~-]+$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var attachmentPath = regexp.MustCompile(`^(?:/releases)?/attachments/[A-Za-z0-9-]+$`)

type Store struct {
	web, api, token string
	client          *http.Client
}

type Item struct {
	Tag     string    `json:"tag"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	SHA256  string    `json:"sha256"`
	Created time.Time `json:"created_at"`
}

type release struct {
	ID      int64     `json:"id"`
	Tag     string    `json:"tag_name"`
	Body    string    `json:"body"`
	Draft   bool      `json:"draft"`
	Created time.Time `json:"created_at"`
	Assets  []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func New(repository, token string) (*Store, error) {
	u, err := url.Parse(strings.TrimSpace(repository))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return nil, errors.New("introduce la URL HTTPS del repositorio Forgejo")
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
	if len(parts) < 2 {
		return nil, errors.New("la URL debe incluir el propietario y el repositorio Forgejo")
	}
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], ".git")
	for _, part := range parts {
		if !segment.MatchString(part) || part == "." || part == ".." {
			return nil, errors.New("ruta del repositorio Forgejo inválida")
		}
	}
	token = strings.TrimSpace(token)
	if len(token) == 0 || len(token) > 1024 || strings.ContainsAny(token, " \t\r\n") {
		return nil, errors.New("introduce un token válido de Forgejo")
	}
	u.Path = "/" + strings.Join(parts, "/")
	web := u.String()
	repoPath := u.Path
	instancePath := strings.TrimSuffix(repoPath, "/"+strings.Join(parts[len(parts)-2:], "/"))
	u.Path = "/api/v1/repos/" + strings.Join(parts[len(parts)-2:], "/")
	if instancePath != "" {
		u.Path = instancePath + u.Path
	}
	return &Store{web: web, api: u.String(), token: token, client: &http.Client{
		Timeout: 30 * time.Minute,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			// Forgejo puede redirigir la descarga a su propio adjunto. La
			// credencial jamás sale del origen indicado por el administrador.
			if len(via) == 1 && via[0].Method == http.MethodGet &&
				strings.HasPrefix(via[0].URL.Path, repoPath+"/releases/download/") &&
				next.URL.Scheme == "https" && next.URL.Host == via[0].URL.Host &&
				next.URL.User == nil && next.URL.RawPath == "" && next.URL.RawQuery == "" &&
				(isAttachment(next.URL.Path, repoPath) || isAttachment(next.URL.Path, instancePath)) {
				return nil
			}
			return http.ErrUseLastResponse
		},
	}}, nil
}

func isAttachment(path, prefix string) bool {
	suffix, found := strings.CutPrefix(path, prefix)
	return found && attachmentPath.MatchString(suffix)
}

func (s *Store) request(ctx context.Context, method, target, media string, body io.Reader, length int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, errors.New("solicitud Forgejo inválida")
	}
	req.Header.Set("Authorization", "token "+s.token)
	if media != "" {
		req.Header.Set("Content-Type", media)
		req.ContentLength = length
	}
	res, err := s.client.Do(req)
	if err != nil {
		return nil, errors.New("no se pudo conectar con Forgejo")
	}
	return res, nil
}

type statusError int

func (e statusError) Error() string { return fmt.Sprintf("Forgejo respondió HTTP %d", int(e)) }

func (s *Store) json(ctx context.Context, method, path, media string, body io.Reader, length int64, out any) error {
	if !strings.HasPrefix(media, "multipart/") {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}
	res, err := s.request(ctx, method, s.api+path, media, body, length)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return statusError(res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || json.Unmarshal(raw, out) != nil {
		return errors.New("respuesta JSON de Forgejo inválida")
	}
	return nil
}

func (s *Store) checkPrivate(ctx context.Context) error {
	var info struct {
		Private bool `json:"private"`
	}
	if err := s.json(ctx, "GET", "", "", nil, 0, &info); err != nil {
		return err
	}
	if !info.Private {
		return errors.New("el repositorio Forgejo debe ser privado")
	}
	return nil
}

func tagFor(name, digest string) string {
	if name == Profile {
		return "hgw-config-" + digest
	}
	return "hgw-backup-" + digest
}

func validTag(tag string) bool {
	for _, name := range []string{Profile, Backup} {
		prefix := tagFor(name, "")
		if strings.HasPrefix(tag, prefix) && digestPattern.MatchString(strings.TrimPrefix(tag, prefix)) {
			return true
		}
	}
	return false
}

func maxSize(name string) int64 {
	if name == Profile {
		return MaxProfile
	}
	return MaxBackup
}

func (r release) item() (Item, bool) {
	digest := strings.TrimPrefix(r.Body, marker)
	if r.Draft || !strings.HasPrefix(r.Body, marker) || !digestPattern.MatchString(digest) {
		return Item{}, false
	}
	for _, a := range r.Assets {
		if (a.Name == Profile || a.Name == Backup) && r.Tag == tagFor(a.Name, digest) && a.Size > 0 && a.Size <= maxSize(a.Name) {
			return Item{r.Tag, a.Name, a.Size, digest, r.Created}, true
		}
	}
	return Item{}, false
}

func (s *Store) List(ctx context.Context, page int) ([]Item, bool, error) {
	if page < 1 || page > 10000 {
		return nil, false, errors.New("página Forgejo inválida")
	}
	if err := s.checkPrivate(ctx); err != nil {
		return nil, false, err
	}
	var releases []release
	if err := s.json(ctx, "GET", fmt.Sprintf("/releases?limit=20&page=%d", page), "", nil, 0, &releases); err != nil {
		return nil, false, err
	}
	items := make([]Item, 0, len(releases))
	for _, entry := range releases {
		if item, ok := entry.item(); ok {
			items = append(items, item)
		}
	}
	return items, len(releases) == 20, nil
}

func (s *Store) find(ctx context.Context, tag string) (release, error) {
	var r release
	err := s.json(ctx, "GET", "/releases/tags/"+tag, "", nil, 0, &r)
	return r, err
}

// Upload reutiliza la publicación si el mismo archivo se reenvía tras un fallo.
func (s *Store) Upload(ctx context.Context, name string, source io.Reader, size int64, digest string) (Item, error) {
	if (name != Profile && name != Backup) || size <= 0 || size > maxSize(name) || !digestPattern.MatchString(digest) {
		return Item{}, errors.New("archivo cifrado o tamaño incompatible con Forgejo")
	}
	if err := s.checkPrivate(ctx); err != nil {
		return Item{}, err
	}
	tag := tagFor(name, digest)
	r, err := s.find(ctx, tag)
	if err == statusError(http.StatusNotFound) {
		raw, _ := json.Marshal(map[string]string{"tag_name": tag, "name": "Headscale Gateway · " + name, "body": marker + digest})
		err = s.json(ctx, "POST", "/releases", "application/json", bytes.NewReader(raw), int64(len(raw)), &r)
		if err != nil {
			// Otra petición pudo crear la misma publicación en paralelo.
			if existing, findErr := s.find(ctx, tag); findErr == nil {
				r, err = existing, nil
			}
		}
	}
	if err != nil {
		return Item{}, err
	}
	if r.ID <= 0 || r.Tag != tag || r.Body != marker+digest || r.Draft {
		return Item{}, errors.New("la publicación Forgejo no coincide con el archivo")
	}
	if len(r.Assets) != 0 {
		if item, ok := r.item(); ok && item.Size == size {
			return item, nil
		}
		return Item{}, errors.New("la publicación Forgejo contiene un adjunto distinto")
	}

	// Se calcula solo la envoltura multipart en memoria. El archivo va en flujo.
	var framing bytes.Buffer
	writer := multipart.NewWriter(&framing)
	if _, err := writer.CreateFormFile("attachment", name); err != nil {
		return Item{}, err
	}
	prefixSize := framing.Len()
	if err := writer.Close(); err != nil {
		return Item{}, err
	}
	body := io.MultiReader(bytes.NewReader(framing.Bytes()[:prefixSize]), source, bytes.NewReader(framing.Bytes()[prefixSize:]))
	var asset struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	err = s.json(ctx, "POST", fmt.Sprintf("/releases/%d/assets?name=%s", r.ID, name), writer.FormDataContentType(), body, int64(framing.Len())+size, &asset)
	if err == nil && asset.Name == name && asset.Size == size {
		return Item{tag, name, size, digest, r.Created}, nil
	}
	if stored, findErr := s.find(ctx, tag); findErr == nil {
		if item, ok := stored.item(); ok && item.Size == size {
			return item, nil
		}
	}
	if err != nil {
		return Item{}, err
	}
	return Item{}, errors.New("Forgejo no confirmó el adjunto cifrado")
}

// Download escribe únicamente el tipo solicitado y valida longitud y SHA-256.
func (s *Store) Download(ctx context.Context, tag, name string, destination io.Writer) (Item, error) {
	if (name != Profile && name != Backup) || !validTag(tag) || !strings.HasPrefix(tag, tagFor(name, "")) {
		return Item{}, errors.New("identificador Forgejo inválido")
	}
	if err := s.checkPrivate(ctx); err != nil {
		return Item{}, err
	}
	r, err := s.find(ctx, tag)
	if err != nil {
		return Item{}, err
	}
	item, ok := r.item()
	if !ok || item.Tag != tag || item.Name != name {
		return Item{}, errors.New("la publicación no contiene el archivo cifrado solicitado")
	}
	res, err := s.request(ctx, "GET", s.web+"/releases/download/"+tag+"/"+name, "", nil, 0)
	if err != nil {
		return Item{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Item{}, statusError(res.StatusCode)
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(res.Body, item.Size+1))
	if err != nil || size != item.Size || hex.EncodeToString(hash.Sum(nil)) != item.SHA256 {
		return Item{}, errors.New("el adjunto Forgejo está incompleto o no coincide con su SHA-256")
	}
	return item, nil
}
