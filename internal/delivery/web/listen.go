package web

import (
	"fmt"
	"io"
	"net"
	"strconv"
)

// UsableIPv4 returns all IPv4 addresses from up, non-loopback interfaces that
// are not link-local, not loopback, and are unicast.
func UsableIPv4(ifaces []InterfaceInfo) []net.IP {
	var result []net.IP
	for _, iface := range ifaces {
		if !iface.Up || iface.Loopback {
			continue
		}
		for _, ip := range iface.IPs {
			v4 := ip.To4()
			if v4 == nil || v4.IsLoopback() || (v4[0] == 169 && v4[1] == 254) || !v4.IsGlobalUnicast() {
				continue
			}
			result = append(result, v4)
		}
	}
	return result
}

// ChooseListenAddr determines the listen address based on the D14 rule.
func ChooseListenAddr(explicit string, loopbackOnly bool, port int, ifaces []InterfaceInfo) (string, error) {
	if explicit != "" {
		_, portStr, err := net.SplitHostPort(explicit)
		if err != nil {
			return "", fmt.Errorf("invalid address %q: %w", explicit, err)
		}
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 0 || p > 65535 {
			return "", fmt.Errorf("invalid port in address %q", explicit)
		}
		return explicit, nil
	}

	if loopbackOnly {
		return fmt.Sprintf("127.0.0.1:%d", port), nil
	}

	if len(UsableIPv4(ifaces)) >= 2 {
		return fmt.Sprintf("0.0.0.0:%d", port), nil
	}

	return fmt.Sprintf("127.0.0.1:%d", port), nil
}

func isLoopbackHost(host string) bool {
	if host == "127.0.0.1" || host == "localhost" || host == "::1" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}

// WriteStartup prints the startup banner and reachable URLs.
func WriteStartup(w io.Writer, boundHost string, port int, token string, ifaces []InterfaceInfo) {
	const warning = "WARNING: The web UI is reachable from other devices over plain HTTP. Anyone who can observe this traffic can read the session token. Use --loopback-only to keep it on this machine.\n"

	if boundHost == "0.0.0.0" {
		io.WriteString(w, warning)
		fmt.Fprintf(w, "Skill Hub web UI: http://127.0.0.1:%d/#token=%s\n", port, token)
		for _, iface := range ifaces {
			if !iface.Up || iface.Loopback {
				continue
			}
			for _, ip := range iface.IPs {
				v4 := ip.To4()
				if v4 == nil || v4.IsLoopback() || (v4[0] == 169 && v4[1] == 254) || !v4.IsGlobalUnicast() {
					continue
				}
				fmt.Fprintf(w, "  %s: http://%s:%d/#token=%s\n", iface.Name, v4.String(), port, token)
			}
		}
		return
	}

	if isLoopbackHost(boundHost) {
		fmt.Fprintf(w, "Skill Hub web UI: http://127.0.0.1:%d/#token=%s\n", port, token)
		return
	}

	io.WriteString(w, warning)
	fmt.Fprintf(w, "Skill Hub web UI: http://%s:%d/#token=%s\n", boundHost, port, token)
}
