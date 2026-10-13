package web

import (
	"fmt"
	"io"
	"net"
	"strconv"

	"github.com/vantt/skillhub-mcp/internal/delivery/hostpolicy"
)

// DefaultPort is the port `skillhub serve web` listens on when --addr names
// none.
const DefaultPort = 7421

// WriteStartup prints the start-up banner: the warning when a listener can
// be reached from other machines, then one URL with the session token per
// address, built from each listener's real port. The first URL stays on this
// machine when a listener allows it; WriteStartup returns it, so the caller
// opens the browser there.
func WriteStartup(w io.Writer, addresses []net.Addr, token string, interfaces []hostpolicy.Interface) string {
	type entry struct{ label, url string }
	link := func(ip, port string) string {
		return "http://" + net.JoinHostPort(ip, port) + "/#token=" + token
	}
	var local, remote []entry
	for _, address := range addresses {
		tcp, ok := address.(*net.TCPAddr)
		if !ok {
			continue
		}
		port := strconv.Itoa(tcp.Port)
		switch {
		case hostpolicy.IsWildcard(address):
			local = append(local, entry{"127.0.0.1", link("127.0.0.1", port)})
		case hostpolicy.IsLoopback(address):
			local = append(local, entry{tcp.IP.String(), link(tcp.IP.String(), port)})
		}
		for _, reachable := range hostpolicy.Reachable(address, interfaces) {
			remote = append(remote, entry{reachable.Label, link(reachable.IP.String(), port)})
		}
	}
	all := append(local, remote...)
	if len(all) == 0 {
		return ""
	}
	if warning := hostpolicy.ExposureWarning(addresses, DefaultPort); warning != "" {
		fmt.Fprintf(w, "WARNING: %s\n", warning)
		fmt.Fprintln(w, "Anyone with this URL can change the skills every connected agent uses.")
	}
	fmt.Fprintf(w, "Skill Hub web UI: %s\n", all[0].url)
	for _, item := range all[1:] {
		fmt.Fprintf(w, "  %s: %s\n", item.label, item.url)
	}
	return all[0].url
}
