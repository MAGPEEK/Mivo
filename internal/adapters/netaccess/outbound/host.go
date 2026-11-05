package outbound

import (
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// ErrHostNotAllowed reports a request to a host outside Options.AllowedHosts or beyond the
// bound on tracked hosts.
var ErrHostNotAllowed = fmt.Errorf("host not allowed: %w", domain.ErrForbidden)

// normalizeHost lowercases a host and strips a trailing dot so limiter, breaker and
// allow-list agree on one key. Non-ASCII (IDN) hosts are rejected instead of converted to
// punycode: every ADR 0031 host is ASCII and converting would need golang.org/x/net.
func normalizeHost(host string) (string, bool) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return "", false
	}

	for i := range len(host) {
		if host[i] >= 0x80 { //nolint:mnd // ASCII range limit
			return "", false
		}
	}

	return host, true
}

// normalizeRates returns a copy of rates keyed by normalised host.
func normalizeRates(rates map[string]Rate) map[string]Rate {
	normalized := make(map[string]Rate, len(rates))

	for host, rate := range rates {
		if key, ok := normalizeHost(host); ok {
			normalized[key] = rate
		}
	}

	return normalized
}

// hostMatcher checks hosts against an allow-list of exact names and ".domain" suffixes.
type hostMatcher struct {
	exact    map[string]struct{}
	suffixes []string
	enabled  bool
}

func newHostMatcher(allowed []string) hostMatcher {
	matcher := hostMatcher{exact: map[string]struct{}{}, enabled: len(allowed) > 0}

	for _, entry := range allowed {
		host, ok := normalizeHost(entry)
		if !ok {
			continue
		}

		if strings.HasPrefix(host, ".") {
			matcher.suffixes = append(matcher.suffixes, host)
		} else {
			matcher.exact[host] = struct{}{}
		}
	}

	return matcher
}

// allows reports whether host (already normalised) may be contacted. An empty list allows all.
func (m hostMatcher) allows(host string) bool {
	if !m.enabled {
		return true
	}

	if _, ok := m.exact[host]; ok {
		return true
	}

	for _, suffix := range m.suffixes {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}

	return false
}
