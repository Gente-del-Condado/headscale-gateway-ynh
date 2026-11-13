package network

import (
	"reflect"
	"testing"
)

func TestMagicDNSExport(t *testing.T) {
	got, err := BuildMagicDNSRecords("laptop.intra.example.es", []string{"100.101.10.12", "fd7a:115c:a1e0::12", "10.0.0.2", "100.101.10.12"}, []string{"media", "wiki", "media"}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	want := []MagicDNSRecord{
		{"admin.laptop.intra.example.es", "A", "100.101.10.12"},
		{"admin.laptop.intra.example.es", "AAAA", "fd7a:115c:a1e0::12"},
		{"media.laptop.intra.example.es", "A", "100.101.10.12"},
		{"media.laptop.intra.example.es", "AAAA", "fd7a:115c:a1e0::12"},
		{"wiki.laptop.intra.example.es", "A", "100.101.10.12"},
		{"wiki.laptop.intra.example.es", "AAAA", "fd7a:115c:a1e0::12"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	if _, err := BuildMagicDNSRecords("a.example.org", []string{"127.0.0.1"}, []string{"wiki"}, ""); err == nil {
		t.Fatal("accepted non-tailnet IP")
	}
	if _, err := BuildMagicDNSRecords("a.example.org", []string{"100.64.0.5"}, []string{"bad.name"}, ""); err == nil {
		t.Fatal("accepted bad service label")
	}
}
