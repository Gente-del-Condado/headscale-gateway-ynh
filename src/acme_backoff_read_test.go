package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnreadableACMEBackoffDefersIssuance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-database")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	before := time.Now()
	until := readACMEBackoff(Config{DatabasePath: path}, "test.invalid")
	if until.Before(before.Add(50*time.Second)) || until.After(time.Now().Add(70*time.Second)) {
		t.Fatalf("unreadable state must defer issuance: %v", until)
	}
}
