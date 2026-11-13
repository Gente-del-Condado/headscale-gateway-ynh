package diagnostics

import (
	"github.com/gentedelcondado/headscale-gateway/internal/platformfs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeparateChannelsAndTail(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := Configure(dir, 1, 2); err != nil {
		t.Fatal(err)
	}
	defer Close()
	Admin().Print("[HGW-ADMIN-TEST] changes")
	Proxy().Print("[HGW-PROXY-TEST] backend")
	info := Info()
	if info.Directory != dir || info.MaxMB != 1 || info.Backups != 2 {
		t.Fatalf("metadata: %+v", info)
	}
	for _, channel := range []string{"gateway", "admin", "proxy"} {
		stat, err := os.Stat(filepath.Join(dir, channel+".log"))
		if err != nil {
			t.Fatal(err)
		}
		if !platformfs.PrivateFile(stat) {
			t.Fatalf("mode %s=%v", channel, stat.Mode())
		}
	}
	adminLines, err := Tail("admin", 10)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(adminLines, "\n"), "HGW-ADMIN-TEST") {
		t.Fatalf("wrong admin tail: %v", adminLines)
	}
	proxyLines, err := Tail("proxy", 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(proxyLines, "\n"), "HGW-ADMIN-TEST") {
		t.Fatal("channels mixed")
	}
	if _, err = Tail("../../etc/passwd", 10); err == nil {
		t.Fatal("path traversal accepted")
	}
	if _, err = Tail("admin", 201); err == nil {
		t.Fatal("unbounded tail accepted")
	}
}
func TestRedactSecret(t *testing.T) {
	for _, v := range []string{"Authorization: Bearer hgk_hidden", "hskey-api-longsecret", "Bearer e3fe4e"} {
		if strings.Contains(redact(v), "longsecret") || strings.Contains(redact(v), "hgk_hidden") || strings.Contains(redact(v), "e3fe4e") {
			t.Fatalf("leaked %q", v)
		}
	}
}

func TestClearLogsAndContinueWriting(t *testing.T) {
	dir := t.TempDir()
	if err := Configure(dir, 1, 2); err != nil {
		t.Fatal(err)
	}
	defer Close()
	Admin().Print("before")
	for _, name := range channelNames {
		if err := os.WriteFile(filepath.Join(dir, name+".log.1"), []byte("rotated"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	for _, name := range channelNames {
		lines, err := Tail(name, 100)
		if err != nil || len(lines) != 0 {
			t.Fatal("not cleared", name, err)
		}
		if _, err := os.Stat(filepath.Join(dir, name+".log.1")); !os.IsNotExist(err) {
			t.Fatal("rotation remains")
		}
	}
	Admin().Print("after")
	lines, err := Tail("admin", 100)
	if err != nil || len(lines) != 1 || !strings.Contains(lines[0], "after") {
		t.Fatal("writer broken", lines, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "unrelated.txt")); err != nil {
		t.Fatal("unrelated file removed")
	}
}
