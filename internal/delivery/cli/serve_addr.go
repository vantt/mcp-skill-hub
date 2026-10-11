package cli

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
)

// maxListenAddresses bounds the --addr list of `skillhub serve`.
const maxListenAddresses = 3

// listenAddress is one parsed --addr entry.
type listenAddress struct {
	// host is a normalized IP literal, or "" for every interface.
	host string
	port int
}

func (address listenAddress) String() string {
	return net.JoinHostPort(address.host, strconv.Itoa(address.port))
}

// wildcard reports whether the address listens on every interface.
func (address listenAddress) wildcard() bool {
	if address.host == "" {
		return true
	}
	ip, err := netip.ParseAddr(address.host)
	return err == nil && ip.IsUnspecified()
}

// network is the network passed to net.Listen: 0.0.0.0 and IPv4 addresses
// listen on IPv4 only, a specific IPv6 address on IPv6 only, and [::] or an
// empty host on both families where the system allows it.
func (address listenAddress) network() string {
	if address.host == "" {
		return "tcp"
	}
	ip, err := netip.ParseAddr(address.host)
	switch {
	case err != nil:
		return "tcp"
	case ip.Is4():
		return "tcp4"
	case ip.IsUnspecified():
		return "tcp"
	}
	return "tcp6"
}

// parseListenAddresses parses the --addr value of `skillhub serve`: one to
// three comma-separated entries, each an IP address with an optional port
// (IPv6 in brackets, for example [::1]:7420), ":port" or "localhost". A
// missing port takes defaultPort. Two entries that name the same address and
// port, or a wildcard entry (0.0.0.0, [::] or an empty host) next to another
// entry on the same port, are refused. Port 0 asks the system for a free port
// per entry, so it is never a duplicate.
func parseListenAddresses(raw string, defaultPort int) ([]listenAddress, error) {
	entries := strings.Split(raw, ",")
	if len(entries) > maxListenAddresses {
		return nil, fmt.Errorf("--addr takes at most %d addresses, got %d", maxListenAddresses, len(entries))
	}
	addresses := make([]listenAddress, 0, len(entries))
	for _, entry := range entries {
		address, err := parseListenAddress(strings.TrimSpace(entry), defaultPort)
		if err != nil {
			return nil, err
		}
		for _, earlier := range addresses {
			if address.port == 0 || earlier.port != address.port {
				continue
			}
			if earlier.host == address.host {
				return nil, fmt.Errorf("--addr lists %s twice", address)
			}
			if earlier.wildcard() || address.wildcard() {
				return nil, fmt.Errorf("--addr %s and %s overlap: an address on every interface already covers the other on port %d", earlier, address, address.port)
			}
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func parseListenAddress(entry string, defaultPort int) (listenAddress, error) {
	invalid := func(reason string) (listenAddress, error) {
		return listenAddress{}, fmt.Errorf("--addr %q is not usable: %s", entry, reason)
	}
	if entry == "" {
		return invalid("an address is empty; separate addresses with single commas")
	}
	host, portText := entry, ""
	switch {
	case strings.HasPrefix(entry, "["):
		if strings.HasSuffix(entry, "]") {
			host = entry[1 : len(entry)-1]
		} else {
			parsedHost, parsedPort, err := net.SplitHostPort(entry)
			if err != nil {
				return invalid("write an IPv6 address as [::1] or [::1]:7420")
			}
			host, portText = parsedHost, parsedPort
			if portText == "" {
				return invalid("the port after the colon is missing")
			}
		}
	case strings.Count(entry, ":") > 1:
		return invalid("write an IPv6 address in brackets, for example [::1]:7420")
	case strings.Contains(entry, ":"):
		parsedHost, parsedPort, err := net.SplitHostPort(entry)
		if err != nil || parsedPort == "" {
			return invalid("use host:port, for example 127.0.0.1:7420")
		}
		host, portText = parsedHost, parsedPort
	}
	port := defaultPort
	if portText != "" {
		value, err := strconv.Atoi(portText)
		if err != nil || value < 0 || value > 65535 || strings.TrimLeft(portText, "0123456789") != "" {
			return invalid("the port must be a number from 0 to 65535")
		}
		port = value
	}
	if host == "" {
		return listenAddress{host: "", port: port}, nil
	}
	if strings.EqualFold(host, "localhost") {
		return listenAddress{host: "127.0.0.1", port: port}, nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return invalid("give an IP address (a host name can stand for several addresses)")
	}
	return listenAddress{host: ip.Unmap().String(), port: port}, nil
}

// bindError names the address that could not be bound and why.
type bindError struct {
	address listenAddress
	err     error
}

// bindListeners opens one listener per address. If any address fails, the
// listeners already opened are closed, so the command binds all or nothing.
func bindListeners(addresses []listenAddress) ([]net.Listener, *bindError) {
	listeners := make([]net.Listener, 0, len(addresses))
	for _, address := range addresses {
		listener, err := net.Listen(address.network(), address.String())
		if err != nil {
			closeListeners(listeners)
			return nil, &bindError{address: address, err: err}
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		_ = listener.Close()
	}
}

func listenerAddresses(listeners []net.Listener) []net.Addr {
	addresses := make([]net.Addr, len(listeners))
	for index, listener := range listeners {
		addresses[index] = listener.Addr()
	}
	return addresses
}

// reason is the system's own words for the bind failure, without the
// operation and address that the message already names.
func (failure *bindError) reason() string {
	var operation *net.OpError
	if errors.As(failure.err, &operation) && operation.Err != nil {
		return operation.Err.Error()
	}
	return failure.err.Error()
}

// portOwnerHint names the command that shows which process owns a port.
func portOwnerHint(port int) string {
	text := strconv.Itoa(port)
	command := "ss -ltnp 'sport = :" + text + "'"
	switch runtime.GOOS {
	case "darwin":
		command = "lsof -nP -iTCP:" + text + " -sTCP:LISTEN"
	case "windows":
		command = "netstat -ano | findstr :" + text
	}
	return "Stop the process that owns the port (find it with `" + command + "`), or pass another --addr."
}
