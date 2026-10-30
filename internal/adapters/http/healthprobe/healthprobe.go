// Package healthprobe asks a running server whether it is healthy. The container
// HEALTHCHECK runs it through `mivo healthcheck`, so the image needs no curl or wget.
package healthprobe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"time"
)

const (
	// DefaultTimeout bounds one probe, connect included. It stays below the timeout of the
	// Docker HEALTHCHECK, so the probe reports its own error instead of being killed.
	DefaultTimeout = 3 * time.Second

	// HealthPath is the health endpoint of the REST API (api/openapi.yaml).
	HealthPath = "/api/v1/health"

	// loopbackHost is probed when the server listens on all interfaces: it is always bound
	// then and never leaves the host or container.
	loopbackHost = "127.0.0.1"

	probeScheme = "http"
)

// ErrUnhealthy marks a server that answered, but not with 200 OK.
var ErrUnhealthy = errors.New("server is not healthy")

// Options are the inputs of Check.
type Options struct {
	// Address is the host:port of the server, see Address.
	Address string
	// Timeout bounds the whole request; zero or negative means DefaultTimeout.
	Timeout time.Duration
}

// Address returns the host:port to probe for a server configured with host and port. An
// empty or unspecified host (0.0.0.0, ::) means all interfaces, so the loopback address is
// probed; a specific host is probed as configured because the server listens only there.
func Address(host string, port int) string {
	if isAllInterfaces(host) {
		host = loopbackHost
	}

	return net.JoinHostPort(host, strconv.Itoa(port))
}

func isAllInterfaces(host string) bool {
	if host == "" {
		return true
	}

	address, err := netip.ParseAddr(host)

	return err == nil && address.IsUnspecified()
}

// Check GETs the health endpoint at options.Address. It returns nil on 200 OK, an error
// wrapping ErrUnhealthy on any other status and the transport error when the server does
// not answer in time or at all.
func Check(ctx context.Context, options Options) error {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := (&url.URL{Scheme: probeScheme, Host: options.Address, Path: HealthPath}).String()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return fmt.Errorf("probe %s: %w", target, err)
	}

	response, err := newClient().Do(request)
	if err != nil {
		return fmt.Errorf("probe %s: %w", target, err)
	}

	// Keep-alives are off, so the body need not be drained; a close error changes nothing
	// about the answer already received.
	_ = response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: status %d: %w", target, response.StatusCode, ErrUnhealthy)
	}

	return nil
}

// newClient returns a client for one request. It ignores proxy variables because the probe
// targets the local server, and it does not follow redirects because a redirect is not a
// health answer.
func newClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
