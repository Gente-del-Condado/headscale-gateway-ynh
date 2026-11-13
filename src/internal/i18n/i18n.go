// El paquete i18n sirve mensajes gettext compilados desde archivos PO editables en Poedit.
// El español es el idioma original y se usa cuando falta una traducción.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
)

//go:embed locales/*.json
var locales embed.FS
var once sync.Once
var dictionaries map[string]map[string]string

func initialize() {
	dictionaries = make(map[string]map[string]string)
	files, err := fs.Glob(locales, "locales/*.json")
	if err != nil {
		return
	}
	for _, file := range files {
		raw, err := locales.ReadFile(file)
		if err != nil {
			continue
		}
		var messages map[string]string
		if json.Unmarshal(raw, &messages) == nil {
			lang := strings.TrimSuffix(strings.TrimPrefix(file, "locales/"), ".json")
			dictionaries[lang] = messages
		}
	}
}
func Codes() []string {
	once.Do(initialize)
	result := make([]string, 0, len(dictionaries))
	for k := range dictionaries {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
func supported(lang string) bool { once.Do(initialize); _, ok := dictionaries[lang]; return ok }
func Language(header string) string {
	for part := range strings.SplitSeq(header, ",") {
		code, _, _ := strings.Cut(part, ";")
		lang := strings.ToLower(strings.TrimSpace(code))
		if supported(lang) {
			return lang
		}
		if index := strings.IndexByte(lang, '-'); index >= 0 && supported(lang[:index]) {
			return lang[:index]
		}
	}
	return "es"
}
func Translate(lang, msg string) string {
	if lang == "es" {
		return msg
	}
	once.Do(initialize)
	if text := dictionaries[lang][msg]; text != "" {
		return text
	}
	return translateFormatted(lang, msg)
}
func T(msg string) string { return Translate(Language(os.Getenv("GATEWAY_LANG")), msg) }
func Data(lang string) ([]byte, error) {
	if !supported(lang) {
		return nil, errors.New("idioma no disponible")
	}
	return locales.ReadFile("locales/" + lang + ".json")
}
