package outbound

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"

	"github.com/magpeek/mivo/internal/domain"
)

// ErrBlockedAddress reports a connection to an address that is not publicly routable.
var ErrBlockedAddress = fmt.Errorf("blocked address: %w", domain.ErrForbidden)

// blockedPrefixes lists special-purpose ranges that IsGlobalUnicast alone accepts. NAT64,
// 6to4, Teredo and IPv4-compatible space are blocked as a whole, which also covers the IPv4
// address embedded in them (a stricter form of re-checking it).
//
//nolint:gochecknoglobals // immutable table, parsed once at package initialisation
var blockedPrefixes = parsePrefixes(
	"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4",
	"::/96", "64:ff9b::/96", "64:ff9b:1::/48", "2002::/16", "2001::/32", "2001:db8::/32",
	"fec0::/10", "fc00::/7",
)

func parsePrefixes(prefixes ...string) []netip.Prefix {
	parsed := make([]netip.Prefix, 0, len(prefixes))
	for _, prefix := range prefixes {
		parsed = append(parsed, netip.MustParsePrefix(prefix))
	}

	return parsed
}

// Resolver resolves host names; *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// isPublic reports whether addr may be contacted. IPv4-mapped IPv6 addresses are judged by
// their IPv4 form so ::ffff:10.0.0.1 cannot slip through.
func isPublic(addr netip.Addr, allowLoopback bool) bool {
	addr = addr.Unmap().WithZone("")
	if !addr.IsValid() {
		return false
	}

	if addr.IsLoopback() {
		return allowLoopback
	}

	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return false
	}

	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}

	return true
}

// guardedDialer resolves a host itself, validates every answer and dials the validated IP
// literal, so a DNS answer cannot change between check and connect (rebinding).
type guardedDialer struct {
	resolver      Resolver
	dialer        net.Dialer
	allowLoopback bool
}

func (g *guardedDialer) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("split dial address %q: %w", address, err)
	}

	addrs, err := g.resolve(ctx, host)
	if err != nil {
		return nil, err
	}

	var dialErr error

	for _, addr := range addrs {
		conn, err := g.dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
		if err == nil {
			return conn, nil
		}

		dialErr = errors.Join(dialErr, err)
	}

	return nil, fmt.Errorf("dial %s: %w", host, dialErr)
}

func (g *guardedDialer) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	addrs, err := g.lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}

	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve %s: no addresses: %w", host, domain.ErrUnavailable)
	}

	for _, addr := range addrs {
		if !isPublic(addr, g.allowLoopback) {
			return nil, fmt.Errorf("%w: %s resolves to a non-public address", ErrBlockedAddress, host)
		}
	}

	return addrs, nil
}

func (g *guardedDialer) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	if literal, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{literal}, nil
	}

	return g.resolver.LookupNetIP(ctx, "ip", host)
}
