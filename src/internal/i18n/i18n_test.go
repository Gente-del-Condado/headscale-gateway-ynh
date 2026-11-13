package i18n

import "testing"

func TestLocalization(t *testing.T) {
	if Language("en-US,en;q=0.9,es;q=0.5") != "en" {
		t.Fatal("idioma inglés")
	}
	if Language("fr-FR") != "es" {
		t.Fatal("español predeterminado")
	}
	if Translate("en", "Plantillas de proxy") != "Proxy templates" {
		t.Fatal("catálogo inglés no cargado")
	}
	if Translate("en", "sin traducción") != "sin traducción" {
		t.Fatal("fallback")
	}
	if _, err := Data("en"); err != nil {
		t.Fatal(err)
	}
}
