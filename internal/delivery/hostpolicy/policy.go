// Package hostpolicy holds the network rules that the two Skill Hub network
// servers share: which Host header values a server accepts, which of this
// machine's addresses a start-up banner prints, and the warning printed when
// a server can be reached from other machines. It imports neither server.
package hostpolicy

import (
	"net"
	"net/netip"
	"strconv"
	"strings"
)

// Options describe one server: the addresses its listeners are bound to (as
// each listener reports them, so port 0 is already resolved), the extra names
// from --allow-host, this machine's host name, and whether the Host header
// must name one of the bound ports.
type Options struct {
	Listeners  []net.Addr
	AllowHosts []string
	Hostname   string
	// RequirePort makes the port part of the check: a Host header without a
	// port, or with a port no listener is bound to, is refused. The web UI
	// sets it; the MCP listener ignores the port, because an SSH tunnel may
	// forward another local port to it.
	RequirePort bool
}

// Policy decides whether a Host header names this server. It guards against
// DNS rebinding, which needs a name: an IP literal cannot be rebound, so on a
// listener bound to every interface any IP literal is accepted, including
// addresses that appear after start (a VPN that connects later). On a
// listener bound to one address only that address and loopback are accepted.
// Names are limited to localhost, this machine's host name (with and without
// ".local") and the --allow-host names.
type Policy struct {
	requirePort bool
	wildcard    bool
	ports       map[int]bool
	boundIPs    map[netip.Addr]bool
	names       map[string]bool
	// exact holds --allow-host entries written with a port ("name:port");
	// they match that host and port only.
	exact map[string]bool
}

// New builds the policy for one server.
func New(options Options) *Policy {
	policy := &Policy{
		requirePort: options.RequirePort,
		ports:       map[int]bool{},
		boundIPs:    map[netip.Addr]bool{},
		names:       map[string]bool{"localhost": true},
		exact:       map[string]bool{},
	}
	for _, address := range options.Listeners {
		tcp, ok := address.(*net.TCPAddr)
		if !ok {
			continue
		}
		policy.ports[tcp.Port] = true
		ip, ok := netip.AddrFromSlice(tcp.IP)
		ip = ip.Unmap()
		if !ok || ip.IsUnspecified() {
			policy.wildcard = true
			continue
		}
		policy.boundIPs[ip] = true
	}
	if name := normalizeName(options.Hostname); name != "" {
		policy.names[name] = true
		policy.names[name+".local"] = true
	}
	for _, entry := range options.AllowHosts {
		entry = strings.TrimSpace(entry)
		if host, port, err := net.SplitHostPort(entry); err == nil {
			if name := normalizeName(host); name != "" {
				if options.RequirePort {
					policy.exact[net.JoinHostPort(name, port)] = true
				} else {
					policy.names[name] = true
				}
			}
			continue
		}
		if name := normalizeName(entry); name != "" {
			policy.names[name] = true
		}
	}
	return policy
}

// Allows reports whether rawHost, the Host header of a request, names this
// server.
func (policy *Policy) Allows(rawHost string) bool {
	rawHost = strings.TrimSpace(rawHost)
	host, portText, err := net.SplitHostPort(rawHost)
	if err != nil {
		if policy.requirePort {
			return false
		}
		host, portText = rawHost, ""
	}
	host = normalizeName(host)
	if host == "" {
		return false
	}
	if policy.requirePort {
		if policy.exact[net.JoinHostPort(host, portText)] {
			return true
		}
		port, err := strconv.Atoi(portText)
		if err != nil || !policy.ports[port] {
			return false
		}
	}
	if policy.names[host] {
		return true
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	return ip.IsLoopback() || policy.wildcard || policy.boundIPs[ip]
}

// normalizeName lowercases a host name or IP literal and drops brackets and a
// trailing dot.
func normalizeName(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "[]")
	return strings.TrimSuffix(strings.ToLower(value), ".")
}

// IsWildcard reports whether address is bound to every interface.
func IsWildcard(address net.Addr) bool {
	tcp, ok := address.(*net.TCPAddr)
	return ok && (tcp.IP == nil || tcp.IP.IsUnspecified())
}

// IsLoopback reports whether address is bound to a loopback address only.
func IsLoopback(address net.Addr) bool {
	tcp, ok := address.(*net.TCPAddr)
	return ok && tcp.IP != nil && tcp.IP.IsLoopback()
}

// ExposureWarning is the one warning line a server prints when any of its
// listeners can be reached from other machines, or "" when all of them stay
// on this machine. defaultPort is the mode's default port, used in the
// advice to bind loopback instead.
func ExposureWarning(addresses []net.Addr, defaultPort int) string {
	advice := "; use --addr " + net.JoinHostPort("127.0.0.1", strconv.Itoa(defaultPort)) + " to keep it on this machine."
	exposed := false
	for _, address := range addresses {
		if IsWildcard(address) {
			return "Listening on all network interfaces over plain HTTP" + advice
		}
		if !IsLoopback(address) {
			exposed = true
		}
	}
	if exposed {
		return "Listening on a network address over plain HTTP" + advice
	}
	return ""
}
