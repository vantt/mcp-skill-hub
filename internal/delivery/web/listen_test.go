package web

import (
	"bytes"
	"net"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
)

func TestStartupOutput(t *testing.T) {
	interfaces := []hostpolicy.Interface{
		{Name: "lo", Up: true, Loopback: true, IPs: []net.IP{net.ParseIP("127.0.0.1")}},
		{Name: "eth0", Up: true, IPs: []net.IP{net.ParseIP("192.168.1.10")}},
		{Name: "tailscale0", Up: true, IPs: []net.IP{net.ParseIP("100.101.102.103")}},
		{Name: "docker0", Up: true, IPs: []net.IP{net.ParseIP("172.17.0.1")}},
		{Name: "br-0f3e", Up: true, IPs: []net.IP{net.ParseIP("172.18.0.1")}},
	}
	tcp := func(ip string, port int) net.Addr { return &net.TCPAddr{IP: net.ParseIP(ip), Port: port} }

	t.Run("wildcard", func(t *testing.T) {
		var buf bytes.Buffer
		primary := WriteStartup(&buf, []net.Addr{tcp("0.0.0.0", 7421)}, "test-token", interfaces)
		want := "WARNING: Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7421 to keep it on this machine.\n" +
			"Anyone with this URL can change the skills every connected agent uses.\n" +
			"Skill Hub web UI: http://127.0.0.1:7421/#token=test-token\n" +
			"  eth0: http://192.168.1.10:7421/#token=test-token\n" +
			"  Tailscale: http://100.101.102.103:7421/#token=test-token\n"
		if buf.String() != want {
			t.Fatalf("wildcard banner:\n%s\nwant:\n%s", buf.String(), want)
		}
		if primary != "http://127.0.0.1:7421/#token=test-token" {
			t.Fatalf("primary URL = %q", primary)
		}
	})

	t.Run("loopback listeners with resolved ports", func(t *testing.T) {
		var buf bytes.Buffer
		primary := WriteStartup(&buf, []net.Addr{tcp("127.0.0.1", 50123), tcp("::1", 50124)}, "test-token", interfaces)
		want := "Skill Hub web UI: http://127.0.0.1:50123/#token=test-token\n" +
			"  ::1: http://[::1]:50124/#token=test-token\n"
		if buf.String() != want {
			t.Fatalf("loopback banner:\n%s\nwant:\n%s", buf.String(), want)
		}
		if primary != "http://127.0.0.1:50123/#token=test-token" {
			t.Fatalf("primary URL = %q", primary)
		}
		// The release smoke test reads the fifth field of this line.
		if fields := strings.Fields(strings.SplitN(buf.String(), "\n", 2)[0]); len(fields) != 5 || fields[4] != primary {
			t.Fatalf("first line fields = %q", fields)
		}
	})

	t.Run("specific network address", func(t *testing.T) {
		var buf bytes.Buffer
		primary := WriteStartup(&buf, []net.Addr{tcp("192.168.1.10", 7421)}, "test-token", interfaces)
		want := "WARNING: Listening on a network address over plain HTTP; use --addr 127.0.0.1:7421 to keep it on this machine.\n" +
			"Anyone with this URL can change the skills every connected agent uses.\n" +
			"Skill Hub web UI: http://192.168.1.10:7421/#token=test-token\n"
		if buf.String() != want {
			t.Fatalf("specific banner:\n%s\nwant:\n%s", buf.String(), want)
		}
		if primary != "http://192.168.1.10:7421/#token=test-token" {
			t.Fatalf("primary URL = %q", primary)
		}
	})
}
