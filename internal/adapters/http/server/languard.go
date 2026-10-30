package server

import (
	"log/slog"
	"net/http"
	"net/netip"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/healthprobe"
	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// sharedAddressSpaceBits is the prefix length of 100.64.0.0/10 (RFC 6598).
const sharedAddressSpaceBits = 10

// lanOnlyRefusalDetail is the detail of the 403 problem. It is fixed English text that names
// the setting, so an owner who meets it knows what to change.
const lanOnlyRefusalDetail = "Requests from outside the local network are refused (server.lan_only)."

// lanGuard refuses requests whose socket peer is not in the local network with a 403
// forbidden problem, until accounts exist (OD-10, milestone 7). The health check stays
// reachable for container health checks and monitoring.
//
// Only RemoteAddr counts, which net/http takes from the socket. X-Forwarded-For, X-Real-IP
// and Forwarded are written by the client unless a trusted proxy replaced them, and the
// trusted-proxy setting arrives with milestone 7b; honouring them now would let anyone on
// the internet claim a LAN address. A reverse proxy or tunnel that connects from the LAN or
// the host itself therefore lets every client through, which is the documented trade-off.
type lanGuard struct {
	logger *slog.Logger
	onLink *onLinkPrefixes
}

func newLANGuard(settings chainSettings) lanGuard {
	return lanGuard{
		logger: settings.logger,
		onLink: newOnLinkPrefixes(settings.interfacePrefixes, settings.now, settings.logger),
	}
}

func (guard lanGuard) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if isHealthCheck(request) || guard.admitsPeer(request) {
			next.ServeHTTP(writer, request)

			return
		}

		guard.logger.DebugContext(request.Context(), "request from a public address refused",
			logtext.RequestAttributes(request, slog.String("remote_address", logtext.Truncate(request.RemoteAddr)))...)
		restapi.WriteProblem(writer, lanOnlyProblem())
	})
}

// admitsPeer reports whether the socket peer is in a fixed LAN range or, for global IPv6
// peers, inside an on-link prefix of a host interface: dual-stack home networks give LAN
// clients only global and link-local IPv6 addresses, and browsers prefer the global one.
func (guard lanGuard) admitsPeer(request *http.Request) bool {
	address, ok := peerAddress(request.RemoteAddr)
	if !ok {
		return false
	}

	if isLANAddress(address) {
		return true
	}

	// Only IPv6 peers ask for the interface prefixes, so IPv4 scans cost no lookup.
	return address.Is6() && guard.onLink.contains(request.Context(), address)
}

func lanOnlyProblem() apigen.Problem {
	problem := restapi.NewProblem(http.StatusForbidden, restapi.CodeForbidden)
	detail := lanOnlyRefusalDetail
	problem.Detail = &detail

	return problem
}

// isHealthCheck reports a GET or HEAD of exactly the health endpoint. The escaped path is
// compared, so encoded variants such as /api%2Fv1%2Fhealth or /api/v1/heal%74h stay
// guarded: an exemption must not depend on how the router decodes a path.
func isHealthCheck(request *http.Request) bool {
	isRead := request.Method == http.MethodGet || request.Method == http.MethodHead

	return isRead && request.URL.EscapedPath() == healthprobe.HealthPath
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

// isLANAddress reports a loopback, RFC 1918, RFC 6598 (carrier-grade NAT, Tailscale),
// link-local or IPv6 unique local address.
func isLANAddress(address netip.Addr) bool {
	// IsPrivate covers RFC 1918 and fc00::/7; IsLinkLocalUnicast 169.254.0.0/16 and fe80::/10.
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() ||
		sharedAddressSpace().Contains(address)
}

// sharedAddressSpace returns 100.64.0.0/10 (RFC 6598): the address range of Tailscale, whose
// peers are normally the owner's own devices. It is kept per OD-10 despite its costs: ISP
// carrier-grade-NAT neighbours when the router's WAN side sits in shared space with a port
// forward, devices shared into the tailnet from other tailnets, and cloud or Kubernetes
// networks that use the range (ADR 0018, Notes M2-31).
func sharedAddressSpace() netip.Prefix {
	return netip.PrefixFrom(netip.AddrFrom4([4]byte{100, 64, 0, 0}), sharedAddressSpaceBits)
}
