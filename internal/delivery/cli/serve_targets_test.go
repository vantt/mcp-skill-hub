package cli

import (
	"net"
	"reflect"
	"testing"
)

func TestConnectTargetsFromAWildcardListener(t *testing.T) {
	got := connectTargets([]net.Addr{&net.TCPAddr{IP: net.IPv4zero, Port: 7420}}, bannerInterfaces)
	want := []connectTarget{
		{label: "eth0", host: "192.168.1.20", machine: "192.168.1.20", hubURL: "http://192.168.1.20:7420", kind: targetLAN, port: 7420},
		{label: "Tailscale", host: "100.101.102.103", machine: "100.101.102.103", hubURL: "http://100.101.102.103:7420", kind: targetTailscale, port: 7420},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets %+v, want %+v", got, want)
	}
}

func TestConnectTargetsFromALoopbackListener(t *testing.T) {
	got := connectTargets([]net.Addr{&net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 7421}}, bannerInterfaces)
	want := []connectTarget{
		{label: "This machine only", host: "127.0.0.1", machine: "<this-machine>", hubURL: "http://127.0.0.1:7421", kind: targetLoopback, port: 7421},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets %+v, want %+v", got, want)
	}
}

func TestConnectTargetsFromAWildcardWithoutAddressesFallBackToLoopback(t *testing.T) {
	got := connectTargets([]net.Addr{&net.TCPAddr{IP: net.IPv6unspecified, Port: 7420}}, bannerInterfaces[:1])
	if len(got) != 1 || got[0].kind != targetLoopback || got[0].hubURL != "http://127.0.0.1:7420" {
		t.Fatalf("targets %+v", got)
	}
}

func TestConnectTargetsBracketTheMachineOfAnIPv6Address(t *testing.T) {
	got := connectTargets([]net.Addr{&net.TCPAddr{IP: net.ParseIP("2001:db8::5"), Port: 7420}}, bannerInterfaces)
	if len(got) != 1 || got[0].machine != "[2001:db8::5]" || got[0].hubURL != "http://[2001:db8::5]:7420" || got[0].kind != targetLAN {
		t.Fatalf("targets %+v", got)
	}
}
