package diagnostics

// Los canales son fijos: el cliente web no puede aportar una ruta de archivo.
import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var channelNames = []string{"gateway", "proxy", "admin"}

type channelSink struct {
	mu     sync.RWMutex
	writer *rotatingWriter
}

func (c *channelSink) Write(p []byte) (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.writer == nil {
		return len(p), nil
	}
	return c.writer.Write(p)
}
func (c *channelSink) swap(w *rotatingWriter) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.writer
	c.writer = w
	if old != nil {
		return old.file.Close()
	}
	return nil
}

var channels = map[string]*channelSink{"gateway": {}, "proxy": {}, "admin": {}}
var gatewayLogger = log.New(redactedWriter{io.MultiWriter(os.Stderr, channels["gateway"])}, "[HGW] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
var adminLogger = log.New(redactedWriter{io.MultiWriter(os.Stderr, channels["admin"])}, "[HGW-ADMIN] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
var proxyLogger = log.New(redactedWriter{io.MultiWriter(os.Stderr, channels["proxy"])}, "[HGW-PROXY] ", log.LstdFlags|log.Lmicroseconds|log.LUTC)
var configurationMu sync.RWMutex
var configuredDir string
var configuredMB, configuredBackups int

func Admin() *log.Logger { return adminLogger }
func Proxy() *log.Logger { return proxyLogger }

// Configure crea un archivo con rotación por tamaño para cada categoría.
// Los ajustes se aplican atómicamente por canal; los descriptores son privados.
func Configure(dir string, maxMB, backups int) error {
	if maxMB < 1 || maxMB > 1024 || backups < 1 || backups > 20 {
		return errors.New("límites de logs inválidos (MiB 1-1024, copias 1-20)")
	}
	if dir == "" {
		return errors.New("directorio de logs vacío")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	opened := make(map[string]*rotatingWriter)
	for _, name := range channelNames {
		w := &rotatingWriter{path: filepath.Join(dir, name+".log"), maxBytes: int64(maxMB) * 1024 * 1024, backups: backups}
		if err = w.reopen(); err != nil {
			for _, f := range opened {
				_ = f.file.Close()
			}
			return fmt.Errorf("abrir log %s: %w", name, err)
		}
		opened[name] = w
	}
	configurationMu.Lock()
	defer configurationMu.Unlock()
	for _, name := range channelNames {
		if e := channels[name].swap(opened[name]); e != nil {
			Emit("gateway", Error, "HGW-LOG-CLOSE", "No se pudo cerrar el registro anterior: %v", e)
		}
	}
	configuredDir = dir
	configuredMB = maxMB
	configuredBackups = backups
	log.SetOutput(externalWriter{})
	log.SetPrefix("")
	log.SetFlags(0)
	return nil
}

type InfoData struct {
	Level          string   `json:"level"`
	EffectiveLevel string   `json:"effective_level"`
	DebugUntil     string   `json:"debug_until,omitempty"`
	Directory      string   `json:"directory"`
	MaxMB          int      `json:"max_mb"`
	Backups        int      `json:"backups"`
	Files          []string `json:"files"`
}

func Info() InfoData {
	configurationMu.RLock()
	defer configurationMu.RUnlock()
	files := make([]string, 0, len(channelNames))
	for _, name := range channelNames {
		sink := channels[name]
		sink.mu.RLock()
		if sink.writer != nil {
			files = append(files, filepath.Base(sink.writer.path))
		} else {
			files = append(files, name+".log")
		}
		sink.mu.RUnlock()
	}
	until := DebugUntil()
	expiry := ""
	if !until.IsZero() {
		expiry = until.Format(time.RFC3339)
	}
	return InfoData{Directory: configuredDir, MaxMB: configuredMB, Backups: configuredBackups, Files: files, Level: BaseLevel(), EffectiveLevel: EffectiveLevel().Code(), DebugUntil: expiry}
}

var secretPattern = regexp.MustCompile(`(?i)(hskey-api-[a-zA-Z0-9_-]+|Bearer\s+[a-zA-Z0-9._~-]+|hgk_[a-zA-Z0-9_-]+)`)

var sensitiveAssignment = regexp.MustCompile(`(?i)(\b(?:client_secret|api[_-]?key|password|passwd|access_token|refresh_token|token|authorization|secret)\b\s*[:=]\s*["']?)[^\s"'&,;]+`)

// El escritor protege tanto stdout/journald como los archivos: no basta
// con redactar el visor, porque los logs de consola también se conservan.
type redactedWriter struct{ io.Writer }

func (w redactedWriter) Write(data []byte) (int, error) {
	clean := []byte(redact(string(data)))
	_, err := w.Writer.Write(clean)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

var bootstrapPattern = regexp.MustCompile(`(?i)((?:Código de recuperación local|Local recovery code)[^:]*:\s*)[a-f0-9]{24,64}`)

func redact(line string) string {
	line = secretPattern.ReplaceAllString(line, "[REDACTED]")
	line = bootstrapPattern.ReplaceAllString(line, "${1}[REDACTED]")
	return sensitiveAssignment.ReplaceAllString(line, "${1}[REDACTED]")
}

func Redact(text string) string { return redact(text) }

// Tail lee una porción final acotada de un registro conocido y nunca acepta
// rutas del llamante. No sigue enlaces simbólicos fuera del directorio configurado.
func Tail(channel string, limit int) ([]string, error) {
	if _, ok := channels[channel]; !ok {
		return nil, errors.New("canal de log desconocido")
	}
	if limit < 1 || limit > 200 {
		return nil, errors.New("límite de líneas inválido")
	}
	configurationMu.RLock()
	defer configurationMu.RUnlock()
	sink := channels[channel]
	sink.mu.RLock()
	path := filepath.Join(configuredDir, channel+".log")
	if sink.writer != nil {
		path = sink.writer.path
	}
	sink.mu.RUnlock()
	return tailFile(path, limit)
}

// TailAt no requiere arrancar el Gateway y solo acepta canales fijos.
func TailAt(directory, channel string, limit int) ([]string, error) {
	if _, ok := channels[channel]; !ok {
		return nil, errors.New("canal de log desconocido")
	}
	if limit < 1 || limit > 200 {
		return nil, errors.New("límite de líneas inválido")
	}
	if err := ValidateDir(directory); err != nil {
		return nil, err
	}
	return tailFile(filepath.Join(directory, channel+".log"), limit)
}

func tailFile(path string, limit int) ([]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("archivo de log no es regular")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	start := info.Size() - 512*1024
	if start < 0 {
		start = 0
	}
	if _, err = f.Seek(start, io.SeekStart); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lines := make([]string, 0, limit)
	if start > 0 {
		scanner.Scan()
	} // Skip potentially truncated first line.
	for scanner.Scan() {
		lines = append(lines, redact(scanner.Text()))
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func Close() error {
	configurationMu.Lock()
	defer configurationMu.Unlock()
	var result error
	for _, name := range channelNames {
		if err := channels[name].swap(nil); err != nil {
			result = errors.Join(result, err)
		}
	}
	configuredDir = ""
	return result
}

// ValidateDir evita la raíz del sistema y rutas del navegador, salvo elección
// consciente de una ruta local por el administrador del anfitrión.
func ValidateDir(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) == string(filepath.Separator) || strings.ContainsRune(dir, '\x00') {
		return errors.New("la ruta de logs debe ser absoluta y no ser la raíz")
	}
	return nil
}

// Clear elimina solo los tres canales gestionados y sus rotaciones numeradas.
// Journald y la auditoría SQLite independiente no son archivos de este registro.
func Clear() error {
	configurationMu.Lock()
	defer configurationMu.Unlock()
	var result error
	for _, name := range channelNames {
		sink := channels[name]
		sink.mu.Lock()
		if w := sink.writer; w != nil {
			w.mu.Lock()
			if err := w.truncate(); err != nil {
				result = errors.Join(result, err)
			} else {
				w.size = 0
			}
			for n := 1; n <= 20; n++ {
				if err := os.Remove(fmt.Sprintf("%s.%d", w.path, n)); err != nil && !os.IsNotExist(err) {
					result = errors.Join(result, err)
				}
			}
			w.mu.Unlock()
		}
		sink.mu.Unlock()
	}
	return result
}

// La salida estándar externa tiene su propio evento; su texto nunca determina la gravedad.
type externalWriter struct{}

func (externalWriter) Write(p []byte) (int, error) {
	Emit("gateway", InfoLevel, "HGW-EXTERNAL-STDLOG", "Detalle de biblioteca externa: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}
