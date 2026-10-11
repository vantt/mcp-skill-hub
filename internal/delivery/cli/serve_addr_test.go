package cli

import (
	"slices"
	"strings"
	"testing"
)

func TestParseListenAddresses(t *testing.T) {
	t.Parallel()
	addresses := func(t *testing.T, raw string, port int) []string {
		t.Helper()
		parsed, err := parseListenAddresses(raw, port)
		if err != nil {
			t.Fatalf("parseListenAddresses(%q) = %v", raw, err)
		}
		var result []string
		for _, address := range parsed {
			result = append(result, address.network()+" "+address.String())
		}
		return result
	}
	accepted := map[string][]string{
		"0.0.0.0":                       {"tcp4 0.0.0.0:7420"},
		"127.0.0.1":                     {"tcp4 127.0.0.1:7420"},
		"100.64.1.2":                    {"tcp4 100.64.1.2:7420"},
		"localhost:9000":                {"tcp4 127.0.0.1:9000"},
		"[::1]:7420":                    {"tcp6 [::1]:7420"},
		"[::1]":                         {"tcp6 [::1]:7420"},
		"[::]":                          {"tcp [::]:7420"},
		":8000":                         {"tcp :8000"},
		"127.0.0.1:0, 127.0.0.1:0":      {"tcp4 127.0.0.1:0", "tcp4 127.0.0.1:0"},
		"0.0.0.0:0,127.0.0.1:0":         {"tcp4 0.0.0.0:0", "tcp4 127.0.0.1:0"},
		"127.0.0.1,100.64.1.2,[::1]":    {"tcp4 127.0.0.1:7420", "tcp4 100.64.1.2:7420", "tcp6 [::1]:7420"},
		"0.0.0.0:7420,127.0.0.1:7421":   {"tcp4 0.0.0.0:7420", "tcp4 127.0.0.1:7421"},
		"[::ffff:127.0.0.1]:7420":       {"tcp4 127.0.0.1:7420"},
		"[2001:DB8::1]:7420,[::1]:7420": {"tcp6 [2001:db8::1]:7420", "tcp6 [::1]:7420"},
	}
	for raw, want := range accepted {
		if got := addresses(t, raw, 7420); !slices.Equal(got, want) {
			t.Errorf("parseListenAddresses(%q) = %q, want %q", raw, got, want)
		}
	}
	refused := map[string]string{
		"":                         "an address is empty",
		"127.0.0.1,,127.0.0.2":     "an address is empty",
		"a,b,c,d":                  "at most 3 addresses",
		"::1":                      "IPv6 address in brackets",
		"[::1":                     "write an IPv6 address as",
		"[::1]:":                   "the port after the colon is missing",
		"127.0.0.1:":               "use host:port",
		"127.0.0.1:http":           "the port must be a number",
		"127.0.0.1:+80":            "the port must be a number",
		"127.0.0.1:65536":          "the port must be a number",
		"hub.example":              "give an IP address",
		"127.0.0.1:7420,127.0.0.1": "lists 127.0.0.1:7420 twice",
		"localhost,127.0.0.1":      "lists 127.0.0.1:7420 twice",
		"0.0.0.0,0.0.0.0":          "lists 0.0.0.0:7420 twice",
		"0.0.0.0:7420,[::]:7420":   "overlap",
		"0.0.0.0,100.64.1.2":       "overlap",
		"100.64.1.2:9000,:9000":    "overlap",
		"[::]:7420,127.0.0.1:7420": "overlap",
	}
	for raw, want := range refused {
		if _, err := parseListenAddresses(raw, 7420); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("parseListenAddresses(%q) = %v, want an error with %q", raw, err, want)
		}
	}
}

func TestServeDefaultAddresses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		args []string
		want string
	}{
		{nil, "tcp4 0.0.0.0:7420"},
		{[]string{"mcp"}, "tcp4 0.0.0.0:7420"},
		{[]string{"web"}, "tcp4 0.0.0.0:7421"},
		{[]string{"web", "--dev"}, "tcp4 127.0.0.1:7421"},
		{[]string{"--addr", "100.64.1.2"}, "tcp4 100.64.1.2:7420"},
		{[]string{"web", "--addr", "[::1]"}, "tcp6 [::1]:7421"},
	}
	for _, tc := range cases {
		cfg, reason, _ := parseServeArgs(tc.args)
		if reason != "" {
			t.Fatalf("parseServeArgs(%q): %s", tc.args, reason)
		}
		addresses, reason, _ := listenAddressesFor(cfg)
		if reason != "" || len(addresses) != 1 {
			t.Fatalf("listenAddressesFor(%q) = %v, %q", tc.args, addresses, reason)
		}
		if got := addresses[0].network() + " " + addresses[0].String(); got != tc.want {
			t.Errorf("serve %q listens on %q, want %q", tc.args, got, tc.want)
		}
	}
}
