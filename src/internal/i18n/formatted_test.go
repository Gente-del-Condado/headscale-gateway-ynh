package i18n

import "testing"

func TestFormattedErrorsAndParameters(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"Headscale responde HTTP 403 (comprueba URL y API key)", "Headscale returns HTTP 403 (check URL and API key)"},
		{"CA ejemplo: huella incorrecta", "CA ejemplo: incorrect fingerprint"},
		{"texto de dependencia desconocido", "texto de dependencia desconocido"},
	} {
		if got := Translate("en", tc.source); got != tc.want {
			t.Fatalf("%q != %q", got, tc.want)
		}
	}
}
