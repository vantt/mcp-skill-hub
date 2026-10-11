package hostpolicy

import (
	"net"
	"strings"
)

// Interface is one network interface with its state and addresses.
type Interface struct {
	Name     string
	Up       bool
	Loopback bool
	IPs      []net.IP
}

// SystemInterfaces lists this machine's network interfaces.
func SystemInterfaces() ([]Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]Interface, 0, len(interfaces))
	for _, item := range interfaces {
		info := Interface{
			Name:     item.Name,
			Up:       item.Flags&net.FlagUp != 0,
			Loopback: item.Flags&net.FlagLoopback != 0,
		}
		if addresses, err := item.Addrs(); err == nil {
			for _, address := range addresses {
				if network, ok := address.(*net.IPNet); ok {
					info.IPs = append(info.IPs, network.IP)
				}
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// Address is one address of this machine that a banner prints, with a label
// for people: "Tailscale" for the Tailscale range, otherwise the interface
// name.
type Address struct {
	IP    net.IP
	Label string
}

// virtualPrefixes name the bridges and virtual links of containers and
// virtual machines. Another machine cannot reach this one through them, so
// banners leave them out; the Host check still accepts their addresses.
var virtualPrefixes = []string{"docker", "br-", "veth", "virbr", "cni", "podman"}

// tailscaleRange is the carrier-grade NAT range Tailscale assigns from.
var tailscaleRange = &net.IPNet{IP: net.IPv4(100, 64, 0, 0).To4(), Mask: net.CIDRMask(10, 32)}

func usableInterface(item Interface) bool {
	if !item.Up || item.Loopback {
		return false
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(item.Name, prefix) {
			return false
		}
	}
	return true
}

func label(ip net.IP, name string) string {
	if v4 := ip.To4(); v4 != nil && tailscaleRange.Contains(v4) {
		return "Tailscale"
	}
	return name
}

// Reachable lists the addresses at which another machine can reach a
// listener bound to address. A wildcard listener expands to every usable
// address of an up, non-loopback, non-virtual interface (IPv4 only for
// 0.0.0.0; IPv4 and global IPv6 for [::], which Go binds dual-stack). A
// listener bound to one address yields that address, labelled by its
// interface when it has one. A loopback listener yields nothing: other
// machines reach it only through a tunnel.
func Reachable(address net.Addr, interfaces []Interface) []Address {
	tcp, ok := address.(*net.TCPAddr)
	if !ok || IsLoopback(address) {
		return nil
	}
	if !IsWildcard(address) {
		name := "Address"
		for _, item := range interfaces {
			for _, ip := range item.IPs {
				if ip.Equal(tcp.IP) {
					name = item.Name
				}
			}
		}
		return []Address{{IP: tcp.IP, Label: label(tcp.IP, name)}}
	}
	ipv4Only := tcp.IP != nil && tcp.IP.To4() != nil
	var result []Address
	for _, item := range interfaces {
		if !usableInterface(item) {
			continue
		}
		for _, ip := range item.IPs {
			if !ip.IsGlobalUnicast() || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				result = append(result, Address{IP: v4, Label: label(v4, item.Name)})
			} else if !ipv4Only {
				result = append(result, Address{IP: ip, Label: label(ip, item.Name)})
			}
		}
	}
	return result
}
