// Package clientorigin carries the resolved network origin of a request, its client address
// and scheme, from the HTTP server's resolver to the guards and handlers (ADR 0032 §3
// "Trusted reverse proxies"). It only holds the value: the server package computes it once
// per request, and nothing else reads the socket address or the TLS state.
package clientorigin

import (
	"context"
	"net/netip"
)

// Origin is where a request comes from after the trusted-proxy rules were applied.
type Origin struct {
	// Address is the client address: the socket peer, or the client a trusted proxy named.
	// It is the proxy's own address when ForcedRemote is set, and invalid when the socket
	// address did not parse. IPv4-mapped IPv6 addresses are unmapped.
	Address netip.Addr
	// Proxy is the socket peer when it is a trusted proxy, else invalid.
	Proxy netip.Addr
	// Socket is the socket address as net/http reports it ("ip:port"), kept for log lines.
	Socket string
	// Secure reports an https request: a TLS socket, or X-Forwarded-Proto of a trusted proxy.
	Secure bool
	// ForcedRemote is set when a trusted proxy named no usable client. The request then
	// counts as remote whatever Address says (fail closed).
	ForcedRemote bool
}

type originKey struct{}

type localKey struct{}

// WithLocal stores whether the client is in the local network, as the LAN guard's rules
// decide (ranges and on-link IPv6 prefixes), so a handler can record it without knowing them.
func WithLocal(ctx context.Context, local bool) context.Context {
	return context.WithValue(ctx, localKey{}, local)
}

// IsLocal returns the value stored by WithLocal; without one it is false, the stricter
// reading (remote).
func IsLocal(ctx context.Context) bool {
	local, _ := ctx.Value(localKey{}).(bool)

	return local
}

// With stores the origin of a request.
func With(ctx context.Context, origin Origin) context.Context {
	return context.WithValue(ctx, originKey{}, origin)
}

// From returns the origin stored by With. Without one the zero Origin is returned: no valid
// address and not secure, which every consumer reads as remote and plain http, the stricter
// reading.
func From(ctx context.Context) Origin {
	origin, _ := ctx.Value(originKey{}).(Origin)

	return origin
}
