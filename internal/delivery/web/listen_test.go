package web

import (
	"bytes"
	"net"
	"strings"
	"testing"
)

func TestListenRule(t *testing.T) {
	tests := []struct {
		name         string
		explicit     string
		loopbackOnly bool
		port         int
		ifaces       []InterfaceInfo
		wantAddr     string
		wantErr      bool
	}{
		{
			name:     "zero usable",
			port:     7421,
			ifaces:   []InterfaceInfo{},
			wantAddr: "127.0.0.1:7421",
		},
		{
			name: "one usable",
			port: 7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("192.168.1.100")},
				},
			},
			wantAddr: "127.0.0.1:7421",
		},
		{
			name: "two usable on two interfaces",
			port: 7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("192.168.1.100")},
				},
				{
					Name: "wlan0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("10.0.0.5")},
				},
			},
			wantAddr: "0.0.0.0:7421",
		},
		{
			name: "two IPs on one interface",
			port: 7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs: []net.IP{
						net.ParseIP("192.168.1.100"),
						net.ParseIP("192.168.1.101"),
					},
				},
			},
			wantAddr: "0.0.0.0:7421",
		},
		{
			name: "a down interface with two IPs (counts 0)",
			port: 7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   false,
					IPs: []net.IP{
						net.ParseIP("192.168.1.100"),
						net.ParseIP("192.168.1.101"),
					},
				},
			},
			wantAddr: "127.0.0.1:7421",
		},
		{
			name: "IPv6-only interface",
			port: 7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("fe80::1"), net.ParseIP("2001:db8::1")},
				},
			},
			wantAddr: "127.0.0.1:7421",
		},
		{
			name: "link-local 169.254.1.1 plus one usable (counts 1)",
			port: 7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs: []net.IP{
						net.ParseIP("169.254.1.1"),
						net.ParseIP("10.0.0.5"),
					},
				},
			},
			wantAddr: "127.0.0.1:7421",
		},
		{
			name:         "loopbackOnly with three usable",
			loopbackOnly: true,
			port:         7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.11")},
				},
				{
					Name: "wlan0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("10.0.0.5")},
				},
			},
			wantAddr: "127.0.0.1:7421",
		},
		{
			name:     "explicit 10.0.0.5:8080 with three usable",
			explicit: "10.0.0.5:8080",
			port:     7421,
			ifaces: []InterfaceInfo{
				{
					Name: "eth0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("192.168.1.11")},
				},
				{
					Name: "wlan0",
					Up:   true,
					IPs:  []net.IP{net.ParseIP("10.0.0.5")},
				},
			},
			wantAddr: "10.0.0.5:8080",
		},
		{
			name:     "explicit bad -> error",
			explicit: "bad",
			port:     7421,
			wantErr:  true,
		},
		{
			name:     "explicit 127.0.0.1:0 accepted",
			explicit: "127.0.0.1:0",
			port:     7421,
			wantAddr: "127.0.0.1:0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, err := ChooseListenAddr(tc.explicit, tc.loopbackOnly, tc.port, tc.ifaces)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr != tc.wantAddr {
				t.Errorf("got addr %q, want %q", addr, tc.wantAddr)
			}
		})
	}
}

func TestStartupOutput(t *testing.T) {
	ifaces := []InterfaceInfo{
		{
			Name: "eth0",
			Up:   true,
			IPs:  []net.IP{net.ParseIP("192.168.1.10")},
		},
		{
			Name: "wlan0",
			Up:   true,
			IPs:  []net.IP{net.ParseIP("10.0.0.5")},
		},
	}

	t.Run("wildcard case", func(t *testing.T) {
		var buf bytes.Buffer
		WriteStartup(&buf, "0.0.0.0", 7421, "test-token", ifaces)
		out := buf.String()

		if !strings.Contains(out, "WARNING: The web UI is reachable from other devices over plain HTTP.") {
			t.Errorf("expected warning in wildcard output, got:\n%s", out)
		}
		if !strings.Contains(out, "Skill Hub web UI: http://127.0.0.1:7421/#token=test-token") {
			t.Errorf("expected loopback URL in wildcard output, got:\n%s", out)
		}
		if !strings.Contains(out, "  eth0: http://192.168.1.10:7421/#token=test-token") {
			t.Errorf("expected eth0 URL in wildcard output, got:\n%s", out)
		}
		if !strings.Contains(out, "  wlan0: http://10.0.0.5:7421/#token=test-token") {
			t.Errorf("expected wlan0 URL in wildcard output, got:\n%s", out)
		}
	})

	t.Run("loopback case", func(t *testing.T) {
		var buf bytes.Buffer
		WriteStartup(&buf, "127.0.0.1", 7421, "test-token", ifaces)
		out := buf.String()

		if strings.Contains(out, "WARNING") {
			t.Errorf("did not expect WARNING in loopback output, got:\n%s", out)
		}
		if !strings.Contains(out, "Skill Hub web UI: http://127.0.0.1:7421/#token=test-token") {
			t.Errorf("expected loopback URL, got:\n%s", out)
		}
	})

	t.Run("specific non-loopback IP case", func(t *testing.T) {
		var buf bytes.Buffer
		WriteStartup(&buf, "10.0.0.5", 7421, "test-token", ifaces)
		out := buf.String()

		if !strings.Contains(out, "WARNING: The web UI is reachable from other devices over plain HTTP.") {
			t.Errorf("expected warning in specific IP output, got:\n%s", out)
		}
		if !strings.Contains(out, "Skill Hub web UI: http://10.0.0.5:7421/#token=test-token") {
			t.Errorf("expected specific IP URL, got:\n%s", out)
		}
	})
}
