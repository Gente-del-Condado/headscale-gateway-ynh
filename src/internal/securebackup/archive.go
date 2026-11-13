package securebackup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/platformfs"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MaxFiles = 100000
const MaxFileSize int64 = 50 << 30
const MaxTotalSize int64 = 2 << 40

type Manifest struct {
	Format  int               `json:"format"`
	Created string            `json:"created"`
	Mode    string            `json:"mode"`
	Files   map[string]string `json:"files"`
	// Las instantáneas remotas nunca se aplican silenciosamente a Headscale al restaurar.
	RemoteRestored bool `json:"remote_restored"`
}

func validName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "/") && !strings.Contains(name, "\\") && !strings.ContainsRune(name, 0) && path.Clean(name) == name && name != ".." && !strings.HasPrefix(name, "../") && name != "manifest.json"
}
func validRoot(root string) error {
	info, e := os.Lstat(root)
	if e != nil {
		return e
	}
	if !platformfs.PrivateDirectory(info) {
		return errors.New("directorio inseguro, requiere 0700")
	}
	return nil
}

// Pack relaciona nombres POSIX seguros del archivo con las ubicaciones originales.
// No sigue enlaces ni archivos especiales. Cifra el archivo desde su primer byte.
func Pack(ctx context.Context, dest string, k KeyFile, mode string, src map[string]string) (Manifest, error) {
	m := Manifest{Format: 2, Created: time.Now().UTC().Format(time.RFC3339Nano), Mode: mode, Files: map[string]string{}}
	if len(src) == 0 || len(src) > MaxFiles {
		return m, errors.New("número de archivos fuera de límites")
	}
	names := make([]string, 0, len(src))
	var total int64
	for name, file := range src {
		if !validName(name) {
			return m, fmt.Errorf("nombre inseguro: %q", name)
		}
		fi, e := os.Lstat(file)
		if e != nil {
			return m, e
		}
		if !fi.Mode().IsRegular() || fi.Size() > MaxFileSize {
			return m, errors.New("archivo especial o demasiado grande")
		}
		total += fi.Size()
		if total > MaxTotalSize {
			return m, errors.New("copia demasiado grande")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	f, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return m, e
	}
	complete := false
	defer func() {
		_ = f.Close()
		if !complete {
			_ = os.Remove(dest)
		}
	}()
	enc, e := NewWriter(f, k)
	if e != nil {
		return m, e
	}
	gz := gzip.NewWriter(enc)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		if e = ctx.Err(); e != nil {
			return m, e
		}
		file := src[name]
		fi, e := os.Lstat(file)
		if e != nil {
			return m, e
		}
		if !fi.Mode().IsRegular() {
			return m, errors.New("archivo cambiado durante copia")
		}
		r, e := os.Open(file)
		if e != nil {
			return m, e
		}
		header := &tar.Header{Name: name, Mode: 0600, Size: fi.Size(), ModTime: fi.ModTime(), Typeflag: tar.TypeReg}
		if e = tw.WriteHeader(header); e == nil {
			hash := sha256.New()
			written, e1 := io.CopyN(io.MultiWriter(tw, hash), r, fi.Size())
			e = e1
			if e == nil && written == fi.Size() {
				m.Files[name] = hex.EncodeToString(hash.Sum(nil))
			}
		}
		_ = r.Close()
		if e != nil {
			return m, e
		}
	}
	data, e := json.Marshal(m)
	if e != nil {
		return m, e
	}
	if e = tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Typeflag: tar.TypeReg, Size: int64(len(data))}); e != nil {
		return m, e
	}
	if _, e = tw.Write(data); e != nil {
		return m, e
	}
	if e = tw.Close(); e != nil {
		return m, e
	}
	if e = gz.Close(); e != nil {
		return m, e
	}
	if e = enc.Close(); e != nil {
		return m, e
	}
	if e = f.Sync(); e != nil {
		return m, e
	}
	if e = f.Close(); e != nil {
		return m, e
	}
	complete = true
	return m, nil
}

// Extract descifra y verifica SHA-256 antes de permitir una restauración.
// El llamante debe aportar un directorio nuevo, vacío, 0700 y fuera del data_dir activo.
func Extract(ctx context.Context, archive, password, dest string) (Manifest, error) {
	var manifest Manifest
	if e := validRoot(dest); e != nil {
		return manifest, e
	}
	a, e := os.Open(archive)
	if e != nil {
		return manifest, e
	}
	defer a.Close()
	reader, e := NewReader(a, password)
	if e != nil {
		return manifest, e
	}
	gz, e := gzip.NewReader(reader)
	if e != nil {
		return manifest, e
	}
	defer gz.Close()
	tarR := tar.NewReader(gz)
	seen := map[string]string{}
	var total int64
	count := 0
	foundManifest := false
	for {
		if e = ctx.Err(); e != nil {
			return manifest, e
		}
		h, e := tarR.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return manifest, e
		}
		if !validName(h.Name) && h.Name != "manifest.json" {
			return manifest, errors.New("entrada de copia insegura")
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return manifest, errors.New("copia contiene entradas no regulares")
		}
		if h.Size < 0 || h.Size > MaxFileSize {
			return manifest, errors.New("tamaño de archivo no permitido")
		}
		total += h.Size
		count++
		if count > MaxFiles+1 || total > MaxTotalSize {
			return manifest, errors.New("límite de tamaño de copia superado")
		}
		if h.Name == "manifest.json" {
			if foundManifest || h.Size > 4<<20 {
				return manifest, errors.New("manifiesto duplicado o excesivo")
			}
			raw, e := io.ReadAll(io.LimitReader(tarR, 4<<20))
			if e != nil {
				return manifest, e
			}
			if e = json.Unmarshal(raw, &manifest); e != nil {
				return manifest, e
			}
			foundManifest = true
			continue
		}
		if foundManifest {
			return manifest, errors.New("datos después del manifiesto")
		}
		if _, dup := seen[h.Name]; dup {
			return manifest, errors.New("archivo duplicado")
		}
		target := filepath.Join(dest, filepath.FromSlash(h.Name))
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return manifest, e
		}
		out, e := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return manifest, e
		}
		hash := sha256.New()
		_, e = io.CopyN(io.MultiWriter(out, hash), tarR, h.Size)
		if e == nil {
			e = out.Sync()
		}
		if ce := out.Close(); e == nil {
			e = ce
		}
		if e != nil {
			return manifest, e
		}
		seen[h.Name] = hex.EncodeToString(hash.Sum(nil))
	}
	if _, e = io.Copy(io.Discard, gz); e != nil {
		return manifest, e
	} // Consume authenticated END block.
	if !foundManifest || manifest.Format != 2 || len(manifest.Files) != len(seen) {
		return manifest, errors.New("manifiesto ausente o incoherente")
	}
	for name, sum := range manifest.Files {
		if seen[name] != sum {
			return manifest, fmt.Errorf("fallo SHA-256: %s", name)
		}
	}
	return manifest, nil
}

// ScanDirectory enumerates files without following symlinks, excluding backups.
func ScanDirectory(root string, filter func(string) bool) (map[string]string, error) {
	result := map[string]string{}
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	e = filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "backups" || strings.HasPrefix(rel, "backups/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if filter != nil && !filter(rel) {
			if d.IsDir() {
				return nil
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("enlace simbólico no admitido: %s", rel)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("archivo especial: %s", rel)
		}
		result["data/"+rel] = p
		return nil
	})
	return result, e
}
