package policytemplates

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltinsAndBundleValidation(t *testing.T) {
	for _, item := range Builtins() {
		if err := Validate(item); err != nil {
			t.Fatalf("%s: %v", item.ID, err)
		}
	}
	bundle := Bundle{Format: Format, Version: 1, Templates: Builtins()}
	raw, _ := json.Marshal(bundle)
	parsed, err := Decode(raw)
	if err != nil || len(parsed.Templates) != len(Builtins()) {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	bundle.Templates = append(bundle.Templates, bundle.Templates[0])
	raw, _ = json.Marshal(bundle)
	if _, err := Decode(raw); err == nil {
		t.Fatal("duplicate ID accepted")
	}
	if _, err := Decode([]byte(strings.Repeat("x", MaxCatalogBytes+1))); err == nil {
		t.Fatal("oversize accepted")
	}
}
func TestHuJSONParsing(t *testing.T) {
	obj, err := Normalize("{\n //comment\n \"groups\":{\"group:family\":[\"user@example.org\"],},\n}")
	if err != nil || obj["groups"] == nil {
		t.Fatalf("huJSON comments rejected: %v", err)
	}
	for _, input := range []string{"[]", "", "{\"grants\":[}", "null"} {
		if _, err := Normalize(input); err == nil {
			t.Fatalf("invalid policy accepted: %s", input)
		}
	}
}
