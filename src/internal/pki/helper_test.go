package pki

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestHelperRequestBoundary(t *testing.T) {
	pem := rootForTest(t, true)
	a, err := ParseRoot([]byte(pem))
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(pem))
	for _, action := range []string{"install", "uninstall"} {
		got, root, err := HelperRequest([]string{action, a.Fingerprint, encoded})
		if err != nil || got != action || root.Fingerprint != a.Fingerprint || !root.Approved {
			t.Fatalf("valid request rejected: %v", err)
		}
	}
	for _, args := range [][]string{{"shell", a.Fingerprint, encoded}, {"install", "../root", encoded}, {"install", strings.Repeat("0", 64), encoded}, {"install", a.Fingerprint, "bad"}, {"install", a.Fingerprint, encoded, "extra"}, {"install", a.Fingerprint, strings.Repeat("a", 12001)}, {"install", a.Fingerprint, base64.StdEncoding.EncodeToString([]byte(pem + "PRIVATE KEY"))}, {"install", a.Fingerprint, base64.StdEncoding.EncodeToString([]byte(rootForTest(t, false)))}} {
		if _, _, err := HelperRequest(args); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}
