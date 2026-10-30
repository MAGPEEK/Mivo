package server

import (
	"fmt"
	"net"
	"net/netip"
)

// Bounds of the IPv6 prefixes the LAN guard trusts from the host interfaces.
const (
	// minOnLinkPrefixBits is the shortest interface prefix the guard trusts. A /48 is the
	// largest site an ISP delegates; anything shorter would admit other customers.
	minOnLinkPrefixBits = 48

	// globalUnicastBits is the length of 2000::/3, the global unicast space (RFC 4291).
	globalUnicastBits = 3
	// teredoBits is the length of 2001::/32 (RFC 4380).
	teredoBits = 32
	// sixToFourBits is the length of 2002::/16 (RFC 3056).
	sixToFourBits = 16
)

// InterfacePrefixProvider reports the IP prefixes of the host's network interfaces. The LAN
// guard admits IPv6 peers inside a global unicast prefix of length /48 or longer (see
// Options.InterfacePrefixes); every other prefix it returns is ignored.
type InterfacePrefixProvider interface {
	InterfacePrefixes() ([]netip.Prefix, error)
}

// systemInterfacePrefixes reads the IPv6 prefixes of the host's interfaces that are up and
// not loopback. It is the default InterfacePrefixProvider.
type systemInterfacePrefixes struct{}

// InterfacePrefixes lists the IPv6 network prefixes of every interface that is up.
func (systemInterfacePrefixes) InterfacePrefixes() ([]netip.Prefix, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list network interfaces: %w", err)
	}

	var prefixes []netip.Prefix

	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addresses, err := networkInterface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("addresses of network interface %s: %w", networkInterface.Name, err)
		}

		prefixes = append(prefixes, ipv6NetworkPrefixes(addresses)...)
	}

	return prefixes, nil
}

// ipv6NetworkPrefixes returns the masked IPv6 prefixes of the interface addresses. IPv4
// addresses are left out: the guard never trusts an IPv4 interface subnet, which on a VPS
// would admit its neighbours.
func ipv6NetworkPrefixes(addresses []net.Addr) []netip.Prefix {
	var prefixes []netip.Prefix

	for _, interfaceAddress := range addresses {
		network, ok := interfaceAddress.(*net.IPNet)
		if !ok || network.IP.To4() != nil {
			continue
		}

		address, ok := netip.AddrFromSlice(network.IP)
		ones, bits := network.Mask.Size()

		// Size reports 0, 0 for a non-canonical mask, which names no prefix.
		if !ok || bits == 0 {
			continue
		}

		prefixes = append(prefixes, netip.PrefixFrom(address, ones).Masked())
	}

	return prefixes
}

// trustedOnLinkPrefixes keeps the prefixes of at least minOnLinkPrefixBits inside the global
// unicast space 2000::/3, without the tunnel ranges Teredo and 6to4. Everything else is
// dropped:
//   - IPv4 and IPv4-mapped prefixes: a public IPv4 subnet would admit neighbours on a VPS.
//   - Unique local (fc00::/7), link-local and loopback prefixes: the fixed LAN ranges admit
//     those peers already, so the interface path never needs them.
//   - IPv4-compatible (::/96, Linux sit0), NAT64 (64:ff9b::/96), discard (100::/64) and
//     multicast prefixes: they lie outside 2000::/3 and name no local link.
//   - Teredo (2001::/32): the on-link /64 of a Teredo pseudo-interface (Windows) holds every
//     Teredo client of the same relay, which are hosts on the internet.
//   - 6to4 (2002::/16): its prefixes are derived from public IPv4 addresses of a tunnel.
func trustedOnLinkPrefixes(prefixes []netip.Prefix) []netip.Prefix {
	trusted := make([]netip.Prefix, 0, len(prefixes))

	for _, prefix := range prefixes {
		if isTrustedOnLinkPrefix(prefix) {
			trusted = append(trusted, prefix.Masked())
		}
	}

	return trusted
}

func isTrustedOnLinkPrefix(prefix netip.Prefix) bool {
	// Every trusted prefix is at least a /48, so its address alone tells which of the much
	// larger special ranges it lies in.
	address := prefix.Addr()

	return prefix.IsValid() && prefix.Bits() >= minOnLinkPrefixBits &&
		globalUnicastSpace().Contains(address) &&
		!teredoSpace().Contains(address) &&
		!sixToFourSpace().Contains(address)
}

// globalUnicastSpace returns 2000::/3.
func globalUnicastSpace() netip.Prefix {
	return netip.PrefixFrom(netip.AddrFrom16([16]byte{0x20}), globalUnicastBits)
}

// teredoSpace returns 2001::/32.
func teredoSpace() netip.Prefix {
	return netip.PrefixFrom(netip.AddrFrom16([16]byte{0x20, 0x01}), teredoBits)
}

// sixToFourSpace returns 2002::/16.
func sixToFourSpace() netip.Prefix {
	return netip.PrefixFrom(netip.AddrFrom16([16]byte{0x20, 0x02}), sixToFourBits)
}
