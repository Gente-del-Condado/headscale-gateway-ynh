package pki

import (
	"encoding/json"
	"net"
	"testing"
)

func TestDiscoveryMessagesNotTrust(t *testing.T) {
	data, _ := json.Marshal(discoveryFrame{Magic: discoveryMagic, Type: "peer", Endpoint: "https://admin.pc.intra.example", Name: "pc"})
	got, ok := discoveryReply(data, net.ParseIP("192.168.10.4"))
	if !ok || got.Channel != "lan-udp-unverified" {
		t.Fatalf("%+v %v", got, ok)
	}
	if _, ok = discoveryReply(data, net.ParseIP("8.8.8.8")); ok {
		t.Fatal("aceptó IP pública")
	}
	if _, ok = discoveryReply([]byte(`{"magic":"HGW-PEER-DISCOVERY/1","type":"peer","endpoint":"http://inseguro"}`), net.ParseIP("192.168.1.3")); ok {
		t.Fatal("aceptó HTTP")
	}
}

func TestDiscoveryAdvertisesCustomPorts(t *testing.T) {
	data, _ := json.Marshal(discoveryFrame{Magic: discoveryMagic, Type: "peer", Endpoint: "https://node.example.test:19443", CAPort: 19080})
	got, ok := discoveryReply(data, net.ParseIP("192.168.1.20"))
	if !ok || got.Port != 19443 || got.CAPort != 19080 {
		t.Fatal(got, ok)
	}
	addr, err := resolveDiscoveryGroup(47992)
	if err != nil || addr.Port != 47992 {
		t.Fatal(addr, err)
	}
	for _, bad := range []int{-1, 65536} {
		if _, err := resolveDiscoveryGroup(bad); err == nil {
			t.Fatal("invalid discovery port accepted")
		}
	}
}
