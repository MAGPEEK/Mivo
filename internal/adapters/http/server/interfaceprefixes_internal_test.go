package server

import (
	"net"
	"net/http"
	"net/netip"
	"testing"
)

func prefixes(t *testing.T, values ...string) []netip.Prefix {
	t.Helper()

	parsed := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			t.Fatalf("parse prefix %q: %v", value, err)
		}

		parsed = append(parsed, prefix)
	}

	return parsed
}

func address(t *testing.T, value string) netip.Addr {
	t.Helper()

	parsed, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatalf("parse address %q: %v", value, err)
	}

	return parsed
}

func TestTrustedOnLinkPrefixes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		interfaces []string
		peer       string
		want       bool
	}{
		{name: "peer inside an interface /64", interfaces: []string{"2001:db8:1:2::/64"}, peer: "2001:db8:1:2::abcd", want: true},
		{name: "isp prefix", interfaces: []string{"2a02:810d:4b40:1f00::/64"}, peer: "2a02:810d:4b40:1f00:1c2d::7", want: true},
		{name: "interface address with host bits", interfaces: []string{"2001:db8:1:2::5/64"}, peer: "2001:db8:1:2:aaaa::1", want: true},
		{name: "peer outside the interface /64", interfaces: []string{"2001:db8:1:2::/64"}, peer: "2001:db8:1:3::1", want: false},
		{name: "second interface", interfaces: []string{"2001:db8:1:2::/64", "2001:db8:9::/56"}, peer: "2001:db8:9:ff::1", want: true},
		{name: "a /48 still counts", interfaces: []string{"2001:db8:7::/48"}, peer: "2001:db8:7:ffff::1", want: true},
		{name: "top of 2000::/3", interfaces: []string{"3fff:ffff:ffff::/48"}, peer: "3fff:ffff:ffff::1", want: true},
		// A shorter prefix would admit a whole provider or more.
		{name: "prefix shorter than /48 ignored", interfaces: []string{"2001:db8::/47"}, peer: "2001:db8:1::1", want: false},
		{name: "default route ignored", interfaces: []string{"::/0"}, peer: "2606:4700:4700::1111", want: false},
		// The on-link /64 of a Teredo pseudo-interface holds every client of the relay.
		{name: "teredo /64 ignored", interfaces: []string{"2001:0:5ef5:79fb::/64"}, peer: "2001:0:5ef5:79fb:0:59d:a0e5:ba1", want: false},
		{name: "6to4 /48 ignored", interfaces: []string{"2002:cb00:7107::/48"}, peer: "2002:cb00:7107:1::1", want: false},
		{name: "ipv4-compatible ::/96 ignored (sit0)", interfaces: []string{"::/96"}, peer: "::203.0.113.9", want: false},
		{name: "nat64 64:ff9b::/96 ignored", interfaces: []string{"64:ff9b::/96"}, peer: "64:ff9b::cb00:7109", want: false},
		{name: "discard 100::/64 ignored", interfaces: []string{"100::/64"}, peer: "100::1", want: false},
		{name: "reserved space above 2000::/3 ignored", interfaces: []string{"4000::/48"}, peer: "4000::1", want: false},
		// The fixed LAN ranges admit unique local peers; the interface path never must.
		{name: "unique local prefix ignored", interfaces: []string{"fd00:1:2:3::/64"}, peer: "fd00:1:2:3::1", want: false},
		// Public IPv4 subnets of a VPS would admit its neighbours.
		{name: "ipv4 interface subnet ignored", interfaces: []string{"203.0.113.0/24"}, peer: "203.0.113.9", want: false},
		{name: "mapped ipv4 prefix ignored", interfaces: []string{"::ffff:203.0.113.0/120"}, peer: "::ffff:203.0.113.9", want: false},
		{name: "link-local prefix ignored", interfaces: []string{"fe80::/64"}, peer: "fe80::1", want: false},
		{name: "multicast prefix ignored", interfaces: []string{"ff02::/64"}, peer: "ff02::1", want: false},
		{name: "loopback prefix ignored", interfaces: []string{"::1/128"}, peer: "::1", want: false},
		{name: "no interfaces", peer: "2001:db8:1:2::1", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reported := prefixes(t, test.interfaces...)
			if got := anyContains(trustedOnLinkPrefixes(reported), address(t, test.peer)); got != test.want {
				t.Errorf("peer %s with interfaces %q trusted = %v, want %v", test.peer, test.interfaces, got, test.want)
			}
		})
	}
}

func TestTrustedOnLinkPrefixesDropInvalidPrefix(t *testing.T) {
	t.Parallel()

	if got := trustedOnLinkPrefixes([]netip.Prefix{{}}); len(got) != 0 {
		t.Errorf("trustedOnLinkPrefixes(zero prefix) = %v, want none", got)
	}
}

func anyContains(trusted []netip.Prefix, peer netip.Addr) bool {
	for _, prefix := range trusted {
		if prefix.Contains(peer) {
			return true
		}
	}

	return false
}

func TestNilProviderUsesTheSystemInterfaces(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()

	settings, err := newChainSettings(Options{Logger: logger, APIHandler: http.NotFoundHandler()})
	if err != nil {
		t.Fatalf("newChainSettings: %v", err)
	}

	if _, ok := settings.interfacePrefixes.(systemInterfacePrefixes); !ok {
		t.Errorf("provider = %T, want systemInterfacePrefixes", settings.interfacePrefixes)
	}
}

func TestSystemInterfacePrefixesAreIPv6(t *testing.T) {
	t.Parallel()

	found, err := systemInterfacePrefixes{}.InterfacePrefixes()
	if err != nil {
		t.Fatalf("InterfacePrefixes: %v", err)
	}

	for _, prefix := range found {
		if !prefix.IsValid() || !prefix.Addr().Is6() || prefix.Addr().IsLoopback() {
			t.Errorf("prefix %s, want valid IPv6 prefixes of non-loopback interfaces", prefix)
		}
	}
}

func TestIPv6NetworkPrefixes(t *testing.T) {
	t.Parallel()

	_, ipv6Network, _ := net.ParseCIDR("2001:db8:1:2::5/64")
	_, ipv4Network, _ := net.ParseCIDR("192.168.1.5/24")
	nonCanonicalMask := &net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.IPMask{0xff, 0x00, 0xff, 0x00}}

	got := ipv6NetworkPrefixes([]net.Addr{
		ipv6Network, ipv4Network, nonCanonicalMask,
		&net.IPAddr{IP: net.ParseIP("2001:db8::9")},
		&net.IPNet{IP: net.IP{1, 2, 3}, Mask: net.CIDRMask(64, 128)},
	})

	if want := prefixes(t, "2001:db8:1:2::/64"); len(got) != 1 || got[0] != want[0] {
		t.Errorf("ipv6NetworkPrefixes = %v, want %v", got, want)
	}
}
