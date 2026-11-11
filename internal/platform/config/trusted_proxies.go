package config

import (
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// keyServerTrustedProxies lists the reverse proxies whose forwarding headers the HTTP server
// believes (ADR 0032 §3 "Trusted reverse proxies").
const keyServerTrustedProxies = "server.trusted_proxies"

const (
	// trustedProxiesEnvSeparator separates the entries of the environment variable. It is
	// not the OS path list separator of browse.roots: that is ":" outside Windows and would
	// cut IPv6 addresses apart.
	trustedProxiesEnvSeparator = ","
	// prefixSeparator tells a CIDR prefix from a single address.
	prefixSeparator = "/"
	// mappedPrefixBits is the length of ::ffff:0:0/96, the IPv4-mapped IPv6 range.
	mappedPrefixBits = 96
)

var (
	errNotAddressOrPrefix    = errors.New("not an IP address or CIDR prefix")
	errTrustsEveryAddress    = errors.New("a prefix of length 0 would trust every address")
	errAddressZone           = errors.New("addresses with a zone are not supported")
	errDuplicateTrustedProxy = errors.New("duplicate entry")
)

// applyTrustedProxies parses the raw entries and stores them as a whole. Every entry is kept
// masked and without the IPv4-mapped IPv6 form, so the HTTP server compares plain prefixes.
func applyTrustedProxies(cfg *Config, raws []string, _ string) error {
	proxies := make([]netip.Prefix, 0, len(raws))

	for index, raw := range raws {
		proxy, err := parseTrustedProxy(strings.TrimSpace(raw))
		if err == nil && slices.Contains(proxies, proxy) {
			err = errDuplicateTrustedProxy
		}

		if err != nil {
			return fmt.Errorf("%s: %w: %w: %q", listItemLabel(keyServerTrustedProxies, index), ErrInvalid, err, raw)
		}

		proxies = append(proxies, proxy)
	}

	cfg.Server.TrustedProxies = proxies

	return nil
}

// environmentTrustedProxies applies <PREFIX>_SERVER_TRUSTED_PROXIES, a comma-separated
// list; an empty value means no trusted proxy.
func environmentTrustedProxies(cfg *Config, env paths.Environment) error {
	name := envName(keyServerTrustedProxies)

	raw, ok := env.Lookup(name)
	if !ok {
		return nil
	}

	raws := []string{}
	if strings.TrimSpace(raw) != "" {
		raws = strings.Split(raw, trustedProxiesEnvSeparator)
	}

	if err := applyTrustedProxies(cfg, raws, ""); err != nil {
		return fmt.Errorf("environment variable %s: %w", name, err)
	}

	return nil
}

// parseTrustedProxy parses a CIDR prefix or a single address, which becomes a prefix of its
// full length.
func parseTrustedProxy(raw string) (netip.Prefix, error) {
	prefix, err := parsePrefixOrAddress(raw)
	if err != nil {
		return netip.Prefix{}, err
	}

	if prefix.Addr().Is4In6() {
		// ::ffff:a.b.c.d/n covers IPv4 addresses only from /96 on; shorter prefixes mix both
		// families and have no IPv4 form.
		if prefix.Bits() < mappedPrefixBits {
			return netip.Prefix{}, errNotAddressOrPrefix
		}

		prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-mappedPrefixBits)
	}

	if prefix.Bits() == 0 {
		return netip.Prefix{}, errTrustsEveryAddress
	}

	return prefix.Masked(), nil
}

func parsePrefixOrAddress(raw string) (netip.Prefix, error) {
	if strings.Contains(raw, prefixSeparator) {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, errNotAddressOrPrefix
		}

		return prefix, nil
	}

	address, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, errNotAddressOrPrefix
	}

	if address.Zone() != "" {
		return netip.Prefix{}, errAddressZone
	}

	return netip.PrefixFrom(address, address.BitLen()), nil
}
