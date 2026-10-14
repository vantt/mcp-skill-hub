package hostpolicy

import (
	"net/netip"
	"testing"
)

func TestPairingIP(t *testing.T) {
	for address, want := range map[string]bool{
		"127.0.0.1":         true,
		"::1":               true,
		"::ffff:127.0.0.1":  true,
		"100.101.102.103":   true,
		"::ffff:100.64.0.1": true,
		"100.127.255.255":   true,
		"100.128.0.1":       false,
		"100.63.255.255":    false,
		"192.168.1.20":      false,
		"10.0.0.5":          false,
		"8.8.8.8":           false,
		"fe80::1":           false,
		"fd7a:115c:a1e0::1": false,
		"0.0.0.0":           false,
	} {
		if got := PairingIP(netip.MustParseAddr(address)); got != want {
			t.Errorf("PairingIP(%s) = %v, want %v", address, got, want)
		}
	}
	if PairingIP(netip.Addr{}) {
		t.Error("the zero address was accepted")
	}
}
