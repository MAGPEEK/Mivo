// Package outbound builds the one hardened HTTP client every online metadata provider and
// image download shares (ADR 0031 §7). It enforces HTTPS, an SSRF guard on every dial, a
// response size cap, a redirect limit, per-host rate limiting, retry with backoff and a
// per-host circuit breaker, so those rules exist exactly once. It sends no cookies and no
// telemetry; the only identifying header is a User-Agent with product, version and project URL.
package outbound
