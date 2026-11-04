# M6-04 Outbound client: SSRF guard, rate limiter, backoff, circuit breaker
Owner: mivo-backend
Depends on: none
Files/areas: `internal/adapters/netaccess/outbound/**` (new)

## Goal
One hardened HTTP client factory used by all providers, so rate limits, retries and
safety rules exist once (ADR 0031 §7).

## Acceptance criteria
- [ ] `outbound.New(Options)` returns a client with timeout, max body size, ≤ 3 redirects,
      HTTPS-only (override only through an explicit `AllowInsecureLoopback` option for tests),
      User-Agent `Mivo/<version> (+url)` from `internal/platform/brand` and a version string,
      and no cookie jar.
- [ ] SSRF guard on dial: connections to loopback, private, link-local, unique-local,
      multicast and metadata addresses are refused after DNS resolution and after each
      redirect (table test incl. `127.0.0.1`, `10.0.0.1`, `169.254.169.254`, `::1`, IPv4-mapped
      IPv6, DNS name resolving to private).
- [ ] Per-host token bucket (configurable rate and burst); callers waiting honour
      `context` cancellation.
- [ ] Retry with exponential backoff and jitter on 429/502/503/504 and network errors, max
      attempts configurable, `Retry-After` (seconds and HTTP date) respected; 4xx other than 429
      are not retried. Clock and sleeper injected, tests run without real sleeping.
- [ ] Circuit breaker per host: opens after N consecutive failures, half-open after the cool-
      down; open state returns `ErrUnavailable` immediately.
- [ ] Authorization-bearing query parameters/headers are redacted in returned errors and logs.
- [ ] Tests: `httptest` servers only; `go test -race ./internal/adapters/netaccess/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Define the sentinel errors `ErrUnavailable` and `ErrRateLimited` in `app` (M6-06) or a tiny
shared package the adapters may import; if M6-06 is not merged yet, add them to
`internal/domain` errors with a doc comment and let M6-06 reuse them. This client is also
reused for user-supplied image URLs in the later edit dialog.
