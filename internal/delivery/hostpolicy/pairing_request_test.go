package hostpolicy

import (
	"net"
	"net/http"
	"testing"
)

func tcpAddr(ip string) net.Addr { return &net.TCPAddr{IP: net.ParseIP(ip), Port: 7420} }

func TestPairingAllowedChecksBothEnds(t *testing.T) {
	cases := []struct {
		name          string
		local, remote net.Addr
		want          bool
	}{
		{"loopback both", tcpAddr("127.0.0.1"), tcpAddr("127.0.0.1"), true},
		{"ipv6 loopback", tcpAddr("::1"), tcpAddr("::1"), true},
		{"mapped loopback", tcpAddr("::ffff:127.0.0.1"), tcpAddr("::ffff:127.0.0.1"), true},
		{"tailscale both", tcpAddr("100.101.102.103"), tcpAddr("100.64.0.9"), true},
		{"lan local", tcpAddr("192.168.1.20"), tcpAddr("127.0.0.1"), false},
		{"lan remote", tcpAddr("127.0.0.1"), tcpAddr("192.168.1.20"), false},
		{"public remote", tcpAddr("100.101.102.103"), tcpAddr("203.0.113.5"), false},
		{"nil local", nil, tcpAddr("127.0.0.1"), false},
		{"nil remote", tcpAddr("127.0.0.1"), nil, false},
		{"not tcp", &net.UnixAddr{Name: "x", Net: "unix"}, tcpAddr("127.0.0.1"), false},
		{"nil ip", &net.TCPAddr{}, tcpAddr("127.0.0.1"), false},
	}
	for _, tc := range cases {
		if got := PairingAllowed(tc.local, tc.remote, http.Header{}); got != tc.want {
			t.Errorf("%s: PairingAllowed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPairingAllowedRefusesForwardingHeaders(t *testing.T) {
	for _, name := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Via", "x-forwarded-for"} {
		header := http.Header{}
		header[name] = []string{"1.2.3.4"}
		if PairingAllowed(tcpAddr("127.0.0.1"), tcpAddr("127.0.0.1"), header) {
			t.Errorf("header %q was accepted", name)
		}
	}
	header := http.Header{}
	header.Set("User-Agent", "skillhub")
	if !PairingAllowed(tcpAddr("127.0.0.1"), tcpAddr("127.0.0.1"), header) {
		t.Error("an unrelated header was refused")
	}
}
