package diagnostics

// Every application event supplies its severity and stable identifier.
import (
	"fmt"
	"github.com/gentedelcondado/headscale-gateway/internal/i18n"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	Error Level = iota
	Warn
	InfoLevel
	Debug
	Trace
)

func (l Level) String() string { return l.Label() }

// Code es el valor estable de configuración/API, no la etiqueta visible del registro.
func (l Level) Code() string { return [...]string{"error", "warn", "info", "debug", "trace"}[l] }
func ParseLevel(s string) (Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return Error, nil
	case "warn", "warning", "advertencia":
		return Warn, nil
	case "info", "información", "informacion", "":
		return InfoLevel, nil
	case "debug", "depuración", "depuracion":
		return Debug, nil
	case "trace", "traza":
		return Trace, nil
	default:
		return InfoLevel, fmt.Errorf("nivel de log no válido %q (error|warn|info|debug|trace)", s)
	}
}

var levelState = struct {
	sync.RWMutex
	base       Level
	debugUntil time.Time
}{base: InfoLevel}

func SetLevel(value string) error {
	l, e := ParseLevel(value)
	if e != nil {
		return e
	}
	levelState.Lock()
	levelState.base = l
	levelState.debugUntil = time.Time{}
	levelState.Unlock()
	return nil
}
func BaseLevel() string {
	levelState.RLock()
	defer levelState.RUnlock()
	return levelState.base.Code()
}
func EffectiveLevel() Level {
	levelState.RLock()
	defer levelState.RUnlock()
	if time.Now().Before(levelState.debugUntil) && levelState.base < Debug {
		return Debug
	}
	return levelState.base
}

// EnableTemporaryDebug no modifica la configuración persistente. Se desactiva
// automáticamente al alcanzar la fecha de expiración, incluso sin temporizador.
func EnableTemporaryDebug(minutes int) (time.Time, error) {
	if minutes < 1 || minutes > 60 {
		return time.Time{}, fmt.Errorf("duración de depuración inválida (1-60 minutos)")
	}
	until := time.Now().Add(time.Duration(minutes) * time.Minute)
	levelState.Lock()
	levelState.debugUntil = until
	levelState.Unlock()
	return until, nil
}
func DisableTemporaryDebug() {
	levelState.Lock()
	levelState.debugUntil = time.Time{}
	levelState.Unlock()
}
func DebugUntil() time.Time {
	levelState.RLock()
	defer levelState.RUnlock()
	if !time.Now().Before(levelState.debugUntil) {
		return time.Time{}
	}
	return levelState.debugUntil
}
func Enabled(l Level) bool { return l <= EffectiveLevel() }
func logger(channel string) *log.Logger {
	switch channel {
	case "admin":
		return Admin()
	case "proxy":
		return Proxy()
	default:
		return gatewayLogger
	}
}

// Emit aplica nivel y canal sin añadir datos privados de peticiones.
func Emit(channel string, level Level, id, format string, args ...any) {
	if !Enabled(level) {
		return
	}
	localized := append([]any(nil), args...)
	for i, arg := range localized {
		if err, ok := arg.(error); ok {
			localized[i] = i18n.T(err.Error())
		}
	}
	msg := fmt.Sprintf(i18n.T(format), localized...)
	for _, line := range strings.Split(strings.TrimRight(msg, "\n"), "\n") {
		logger(channel).Printf("[%s] [%s] %s", level.Label(), id, line)
	}
}

func Fatal(id, format string, args ...any) { Emit("gateway", Error, id, format, args...); os.Exit(1) }

// HTTPErrorLogger preserves explicit tagging for net/http's own error stream.
func HTTPErrorLogger(channel string) *log.Logger {
	return log.New(httpErrorWriter{channel: channel}, "", 0)
}

type httpErrorWriter struct{ channel string }

func (w httpErrorWriter) Write(p []byte) (int, error) {
	Emit(w.channel, Error, "HGW-HTTP-SERVER", "Error del servidor HTTP: %s", strings.TrimSpace(string(p)))
	return len(p), nil
}
