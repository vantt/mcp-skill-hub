package hostpolicy

import (
	"net"
	"reflect"
	"testing"
)

func tcp(ip string, port int) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: port}
}

func TestPolicyAllows(t *testing.T) {
	t.Parallel()
	wildcard := []net.Addr{tcp("0.0.0.0", 7420)}
	wildcardV6 := []net.Addr{tcp("::", 7420)}
	specific := []net.Addr{tcp("100.101.102.103", 7420)}
	webWildcard := []net.Addr{tcp("0.0.0.0", 7421), tcp("127.0.0.1", 50001)}
	webSpecific := []net.Addr{tcp("10.0.0.5", 7421)}
	tests := []struct {
		name    string
		options Options
		host    string
		want    bool
	}{
		{"wildcard accepts an IPv4 literal", Options{Listeners: wildcard}, "203.0.113.9:7420", true},
		{"wildcard accepts an IPv6 literal", Options{Listeners: wildcard}, "[2001:db8::7]:7420", true},
		{"wildcard accepts an IPv4 literal without port", Options{Listeners: wildcard}, "192.168.1.5", true},
		{"dual-stack wildcard accepts an IPv4 literal", Options{Listeners: wildcardV6}, "100.64.1.2:7420", true},
		{"wildcard refuses a foreign name", Options{Listeners: wildcard}, "evil.example:7420", false},
		{"wildcard refuses a name that looks like an address", Options{Listeners: wildcard}, "203.0.113.9.evil.example", false},
		{"wildcard refuses a zoned address", Options{Listeners: wildcard}, "[fe80::1%25eth0]:7420", false},
		{"specific bind accepts its address", Options{Listeners: specific}, "100.101.102.103:7420", true},
		{"specific bind refuses another address", Options{Listeners: specific}, "203.0.113.9:7420", false},
		{"specific bind accepts loopback", Options{Listeners: specific}, "127.0.0.1:7420", true},
		{"specific bind accepts IPv6 loopback", Options{Listeners: specific}, "[::1]:7420", true},
		{"localhost is accepted", Options{Listeners: specific}, "LOCALHOST:9", true},
		{"host name is accepted", Options{Listeners: specific, Hostname: "DevBox"}, "devbox:7420", true},
		{"host name with .local is accepted", Options{Listeners: specific, Hostname: "devbox"}, "devbox.local.:7420", true},
		{"another name is refused", Options{Listeners: specific, Hostname: "devbox"}, "devbox.evil:7420", false},
		{"allow-host name is accepted", Options{Listeners: specific, AllowHosts: []string{"hub.tailnet.example"}}, "HUB.tailnet.example.:7420", true},
		{"allow-host with a port is a name for MCP", Options{Listeners: specific, AllowHosts: []string{"hub.lan:9000"}}, "hub.lan:7420", true},
		{"empty Host is refused", Options{Listeners: wildcard}, "", false},
		{"web accepts a bound port", Options{Listeners: webWildcard, RequirePort: true}, "192.168.1.5:7421", true},
		{"web accepts the other bound port", Options{Listeners: webWildcard, RequirePort: true}, "127.0.0.1:50001", true},
		{"web refuses a wrong port", Options{Listeners: webWildcard, RequirePort: true}, "192.168.1.5:9999", false},
		{"web refuses a missing port", Options{Listeners: webWildcard, RequirePort: true}, "127.0.0.1", false},
		{"web refuses another address on a specific bind", Options{Listeners: webSpecific, RequirePort: true}, "10.0.0.9:7421", false},
		{"web accepts the address of a specific bind", Options{Listeners: webSpecific, RequirePort: true}, "10.0.0.5:7421", true},
		{"web allow-host name needs a bound port", Options{Listeners: webSpecific, RequirePort: true, AllowHosts: []string{"skillhub.lan"}}, "skillhub.lan:7421", true},
		{"web allow-host name on a wrong port", Options{Listeners: webSpecific, RequirePort: true, AllowHosts: []string{"skillhub.lan"}}, "skillhub.lan:80", false},
		{"web allow-host with a port matches exactly", Options{Listeners: webSpecific, RequirePort: true, AllowHosts: []string{"proxy.lan:443"}}, "PROXY.lan:443", true},
		{"web allow-host with a port refuses another port", Options{Listeners: webSpecific, RequirePort: true, AllowHosts: []string{"proxy.lan:443"}}, "proxy.lan:7421", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := New(tc.options).Allows(tc.host); got != tc.want {
				t.Fatalf("Allows(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestExposureWarning(t *testing.T) {
	t.Parallel()
	all := "Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine."
	cases := []struct {
		addresses []net.Addr
		want      string
	}{
		{[]net.Addr{tcp("127.0.0.1", 0)}, ""},
		{[]net.Addr{tcp("127.0.0.1", 0), tcp("::1", 0)}, ""},
		{[]net.Addr{tcp("0.0.0.0", 7420)}, all},
		{[]net.Addr{tcp("127.0.0.1", 1), tcp("::", 7420)}, all},
		{[]net.Addr{tcp("100.101.102.103", 7420)}, "Listening on a network address over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine."},
	}
	for _, tc := range cases {
		if got := ExposureWarning(tc.addresses, 7420); got != tc.want {
			t.Errorf("ExposureWarning(%v) = %q, want %q", tc.addresses, got, tc.want)
		}
	}
}

func TestReachableFiltersVirtualInterfaces(t *testing.T) {
	t.Parallel()
	interfaces := []Interface{
		{Name: "lo", Up: true, Loopback: true, IPs: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}},
		{Name: "eth0", Up: true, IPs: []net.IP{net.ParseIP("192.168.1.10"), net.ParseIP("fe80::1"), net.ParseIP("2001:db8::10")}},
		{Name: "tailscale0", Up: true, IPs: []net.IP{net.ParseIP("100.101.102.103")}},
		{Name: "docker0", Up: true, IPs: []net.IP{net.ParseIP("172.17.0.1")}},
		{Name: "br-1a2b3c", Up: true, IPs: []net.IP{net.ParseIP("172.18.0.1")}},
		{Name: "veth12", Up: true, IPs: []net.IP{net.ParseIP("172.19.0.1")}},
		{Name: "virbr0", Up: true, IPs: []net.IP{net.ParseIP("192.168.122.1")}},
		{Name: "cni0", Up: true, IPs: []net.IP{net.ParseIP("10.88.0.1")}},
		{Name: "podman1", Up: true, IPs: []net.IP{net.ParseIP("10.89.0.1")}},
		{Name: "wlan0", Up: false, IPs: []net.IP{net.ParseIP("10.0.0.5")}},
		{Name: "eth1", Up: true, IPs: []net.IP{net.ParseIP("169.254.3.4")}},
	}
	labels := func(addresses []Address) []string {
		var result []string
		for _, address := range addresses {
			result = append(result, address.Label+" "+address.IP.String())
		}
		return result
	}
	if got, want := labels(Reachable(tcp("0.0.0.0", 7420), interfaces)), []string{"eth0 192.168.1.10", "Tailscale 100.101.102.103"}; !reflect.DeepEqual(got, want) {
		t.Errorf("0.0.0.0 = %q, want %q", got, want)
	}
	if got, want := labels(Reachable(tcp("::", 7420), interfaces)), []string{"eth0 192.168.1.10", "eth0 2001:db8::10", "Tailscale 100.101.102.103"}; !reflect.DeepEqual(got, want) {
		t.Errorf("[::] = %q, want %q", got, want)
	}
	if got, want := labels(Reachable(tcp("192.168.1.10", 7420), interfaces)), []string{"eth0 192.168.1.10"}; !reflect.DeepEqual(got, want) {
		t.Errorf("specific = %q, want %q", got, want)
	}
	if got, want := labels(Reachable(tcp("100.101.102.103", 7420), nil)), []string{"Tailscale 100.101.102.103"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tailscale without interfaces = %q, want %q", got, want)
	}
	if got := Reachable(tcp("127.0.0.1", 7420), interfaces); got != nil {
		t.Errorf("loopback = %v, want none", got)
	}
}
