package i18n

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Los errores formateados proceden de plantillas confiables del catálogo.
// Compara la plantilla completa, nunca fragmentos arbitrarios, y conserva los parámetros.
var formatToken = regexp.MustCompile(`%(?:\[[0-9]+\])?[-+# 0]*(?:\d+|\*)?(?:\.(?:\d+|\*))?[a-zA-Z%]`)

type formattedMessage struct {
	pattern     *regexp.Regexp
	translation string
	weight      int
}

var formattedOnce sync.Once
var formattedCatalog map[string][]formattedMessage

func initFormatted() {
	once.Do(initialize)
	formattedCatalog = make(map[string][]formattedMessage)
	for lang, dictionary := range dictionaries {
		for source, target := range dictionary {
			spans := formatToken.FindAllStringIndex(source, -1)
			if len(spans) == 0 {
				continue
			}
			cursor := 0
			weight := 0
			var pattern strings.Builder
			pattern.WriteString("(?s)^")
			for _, span := range spans {
				text := source[cursor:span[0]]
				pattern.WriteString(regexp.QuoteMeta(text))
				weight += len(text)
				if source[span[0]:span[1]] == "%%" {
					pattern.WriteString("%")
				} else {
					pattern.WriteString("(.*?)")
				}
				cursor = span[1]
			}
			pattern.WriteString(regexp.QuoteMeta(source[cursor:]))
			pattern.WriteString("$")
			weight += len(source[cursor:])
			if weight < 8 {
				continue
			}
			formattedCatalog[lang] = append(formattedCatalog[lang], formattedMessage{regexp.MustCompile(pattern.String()), target, weight})
		}
		sort.Slice(formattedCatalog[lang], func(i, j int) bool {
			a, b := formattedCatalog[lang][i], formattedCatalog[lang][j]
			if a.weight == b.weight {
				return a.translation < b.translation
			}
			return a.weight > b.weight
		})
	}
}
func translateFormatted(lang, msg string) string {
	formattedOnce.Do(initFormatted)
	for _, entry := range formattedCatalog[lang] {
		matches := entry.pattern.FindStringSubmatch(msg)
		if matches == nil {
			continue
		}
		index := 0
		return formatToken.ReplaceAllStringFunc(entry.translation, func(token string) string {
			if token == "%%" {
				return "%"
			}
			index++
			if index >= len(matches) {
				return token
			}
			return matches[index]
		})
	}
	return msg
}
