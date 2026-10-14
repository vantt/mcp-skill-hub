package hostpolicy

import "net/netip"

// PairingIP reports whether an invite may travel to or from ip: a loopback
// address (the same machine or an SSH tunnel end) or a Tailscale address,
// whose traffic is encrypted. IPv4-mapped IPv6 addresses count as their IPv4
// form. Every other address, a plain LAN one included, is refused so an invite
// is never sent where it could be read on the wire.
func PairingIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	return ip.Is4() && tailscalePrefix.Contains(ip)
}

// tailscalePrefix is the carrier-grade NAT range Tailscale assigns from.
var tailscalePrefix = netip.MustParsePrefix("100.64.0.0/10")
