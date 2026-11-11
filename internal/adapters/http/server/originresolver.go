package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/clientorigin"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
)

// Forwarding headers of a trusted reverse proxy. X-Forwarded-Host, Forwarded (RFC 7239) and
// X-Real-IP are never read (ADR 0032 §3).
const (
	headerForwardedFor   = "X-Forwarded-For"
	headerForwardedProto = "X-Forwarded-Proto"
	// forwardedSeparator separates the hops of one forwarding header line.
	forwardedSeparator = ","
	// maxForwardedEntries bounds the X-Forwarded-For hops read per request, so a huge header
	// cannot burn CPU. Real chains have a handful of hops; a longer one fails closed.
	maxForwardedEntries = 32
)

var (
	errInvalidTrustedProxy = errors.New("trusted proxy is not a valid, unmapped prefix")
	errTrustsEveryPeer     = errors.New("trusted proxy of prefix length 0 would trust every peer")
	// errInvalidForwardedAddress never leaves the resolver; a hop either parses or not.
	errInvalidForwardedAddress = errors.New("not an address")
)

// originResolver computes the client address and scheme of a request once and stores them
// with clientorigin.With. It is the only place of the HTTP adapters that reads the socket
// address and the TLS state: the LAN guard, the management guard, the authentication guard,
// the same-origin rule and the session cookie all use its result.
//
// Forwarding headers are client-controlled unless a proxy the owner listed replaced them, so
// they count only when the socket peer is inside trusted. With an empty list, the default,
// only the socket counts and nobody can claim a LAN address or https.
type originResolver struct {
	trusted []netip.Prefix
}

// newOriginResolver checks the prefixes of Options.TrustedProxies. The configuration already
// refuses a prefix of length 0; refusing it again keeps a wiring mistake from trusting every
// peer.
func newOriginResolver(proxies []netip.Prefix) (originResolver, error) {
	trusted := make([]netip.Prefix, 0, len(proxies))

	for _, proxy := range proxies {
		switch {
		case !proxy.IsValid() || proxy.Addr().Is4In6():
			return originResolver{}, fmt.Errorf("%w: %s", errInvalidTrustedProxy, proxy)
		case proxy.Bits() == 0:
			return originResolver{}, fmt.Errorf("%w: %s", errTrustsEveryPeer, proxy)
		}

		trusted = append(trusted, proxy.Masked())
	}

	return originResolver{trusted: trusted}, nil
}

func (resolver originResolver) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := clientorigin.With(request.Context(), resolver.resolve(request))
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// resolve applies ADR 0032 §3. From an untrusted peer the socket decides everything. From a
// trusted proxy the scheme is X-Forwarded-Proto and the client is taken from
// X-Forwarded-For; when that names no usable client the request keeps the proxy's address
// and is forced remote, so a proxy that forgets the header locks setup and administration
// instead of opening them.
func (resolver originResolver) resolve(request *http.Request) clientorigin.Origin {
	socket, ok := peerAddress(request.RemoteAddr)
	origin := clientorigin.Origin{Address: socket, Socket: request.RemoteAddr, Secure: request.TLS != nil}

	if !ok || !resolver.trusts(socket) {
		return origin
	}

	origin.Proxy = socket
	origin.Secure = forwardedSecure(request.Header)

	client, ok := resolver.forwardedClient(request.Header)
	if !ok {
		origin.ForcedRemote = true

		return origin
	}

	origin.Address = client

	return origin
}

func (resolver originResolver) trusts(address netip.Addr) bool {
	for _, prefix := range resolver.trusted {
		if prefix.Contains(address) {
			return true
		}
	}

	return false
}

// forwardedClient reads the X-Forwarded-For hops from the right: trusted proxies are skipped
// and the first other hop is the client, because everything left of it was written by that
// client or beyond. When every hop is trusted the left-most one is the client. A hop that
// does not parse before the client is found reports false.
func (resolver originResolver) forwardedClient(header http.Header) (netip.Addr, bool) {
	entries, ok := forwardedEntries(header.Values(headerForwardedFor))
	if !ok {
		return netip.Addr{}, false
	}

	var client netip.Addr

	for index := len(entries) - 1; index >= 0; index-- {
		client, ok = parseForwardedAddress(entries[index])
		if !ok {
			return netip.Addr{}, false
		}

		if !resolver.trusts(client) {
			break
		}
	}

	return client, true
}

// forwardedEntries joins the header lines in order and splits them into hops. No hop at all
// or more than maxForwardedEntries report false; the commas are counted before splitting.
func forwardedEntries(values []string) ([]string, bool) {
	count := 0
	for _, value := range values {
		count += strings.Count(value, forwardedSeparator) + 1
	}

	if count == 0 || count > maxForwardedEntries {
		return nil, false
	}

	entries := make([]string, 0, count)
	for _, value := range values {
		entries = append(entries, strings.Split(value, forwardedSeparator)...)
	}

	return entries, true
}

// parseForwardedAddress parses one X-Forwarded-For hop: an IPv4 or IPv6 address, optionally
// with a port ("203.0.113.7:4711", "[2001:db8::1]:4711") or in brackets. IPv4-mapped IPv6
// addresses become their IPv4 address. "unknown", obfuscated identifiers ("_hidden"), zone
// ids and everything else report false.
func parseForwardedAddress(entry string) (netip.Addr, bool) {
	entry = strings.TrimSpace(entry)

	address, err := netip.ParseAddr(entry)
	if err != nil {
		address, err = bracketedOrPortAddress(entry)
	}

	if err != nil || address.Zone() != "" {
		return netip.Addr{}, false
	}

	return address.Unmap(), true
}

func bracketedOrPortAddress(entry string) (netip.Addr, error) {
	if addressAndPort, err := netip.ParseAddrPort(entry); err == nil {
		return addressAndPort.Addr(), nil
	}

	if !strings.HasPrefix(entry, "[") || !strings.HasSuffix(entry, "]") {
		return netip.Addr{}, errInvalidForwardedAddress
	}

	address, err := netip.ParseAddr(entry[1 : len(entry)-1])
	if err != nil {
		return netip.Addr{}, errInvalidForwardedAddress
	}

	return address, nil
}

// forwardedSecure reports whether the last X-Forwarded-Proto value is https in any case: the
// trusted proxy next to the server appends or overwrites last. No header, an empty value or
// anything else means plain http.
func forwardedSecure(header http.Header) bool {
	values := header.Values(headerForwardedProto)
	if len(values) == 0 {
		return false
	}

	last := values[len(values)-1]
	if cut := strings.LastIndex(last, forwardedSeparator); cut >= 0 {
		last = last[cut+1:]
	}

	return strings.EqualFold(strings.TrimSpace(last), schemeHTTPS)
}

// peerAddress parses remoteAddr, an "ip:port" from the socket. IPv4-mapped IPv6 addresses
// (::ffff:a.b.c.d), which dual-stack listeners report for IPv4 clients, become their IPv4
// address. A malformed address reports false.
func peerAddress(remoteAddr string) (netip.Addr, bool) {
	addressAndPort, err := netip.ParseAddrPort(remoteAddr)
	if err != nil {
		return netip.Addr{}, false
	}

	return addressAndPort.Addr().Unmap(), true
}

// originAttributes are the log attributes of a refused request: the socket address, or
// behind a trusted proxy the resolved client address and the proxy's address. Header values
// are never logged verbatim; the client address is the parsed one.
func originAttributes(request *http.Request) []any {
	origin := clientorigin.From(request.Context())
	if !origin.Proxy.IsValid() || origin.Address == origin.Proxy {
		return []any{slog.String("remote_address", logtext.Truncate(origin.Socket))}
	}

	return []any{
		slog.String("remote_address", origin.Address.String()),
		slog.String("proxy_address", origin.Proxy.String()),
	}
}
