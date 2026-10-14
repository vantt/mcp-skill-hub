package hostpolicy

import (
	"net"
	"net/http"
	"net/netip"
)

// forwardingHeaders are the headers a reverse proxy adds. A request carrying
// any of them was relayed, so the addresses of its connection do not show
// where the invite really travelled.
var forwardingHeaders = []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Via"}

// PairingAllowed reports whether a request may carry an invite: both ends of
// its connection pass PairingIP and no forwarding header is present. It fails
// closed: an address that is not a TCP address, or is missing, is refused.
func PairingAllowed(local, remote net.Addr, header http.Header) bool {
	if !pairingEnd(local) || !pairingEnd(remote) {
		return false
	}
	for key := range header {
		canonical := http.CanonicalHeaderKey(key)
		for _, name := range forwardingHeaders {
			if canonical == name {
				return false
			}
		}
	}
	return true
}

func pairingEnd(address net.Addr) bool {
	tcp, ok := address.(*net.TCPAddr)
	if !ok || tcp == nil {
		return false
	}
	ip, ok := netip.AddrFromSlice(tcp.IP)
	return ok && PairingIP(ip.Unmap())
}
