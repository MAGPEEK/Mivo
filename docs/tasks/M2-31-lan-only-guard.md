# M2-31 LAN-only guard until authentication exists (pending OD-10)
Owner: mivo-backend
Depends on: M2-04, M2-08; **only if OD-10 is accepted**
Files/areas: `internal/adapters/http/server/**` (new `languard.go` + tests),
`internal/platform/config/**` (one key)

## Goal
Until accounts exist (milestone 7), a server that a user accidentally exposes to the internet
(port forwarding, DMZ) does not hand out their media to anyone: requests from public IP addresses
are refused.

## Acceptance criteria
- [ ] Middleware rejects requests whose remote address is not loopback, RFC 1918, RFC 6598
      (100.64.0.0/10, e.g. Tailscale), link-local, or IPv6 ULA/link-local/loopback, with 403
      problem `forbidden` and a debug log line; `X-Forwarded-For` is **ignored** (cannot be
      trusted without a trusted-proxy setting, which arrives with milestone 7b).
- [ ] Config key `server.lan_only` (bool, default `true`, env `<PREFIX>_SERVER_LAN_ONLY`) turns it
      off; the startup log says which mode is active.
- [ ] The guard covers every route except `GET /api/v1/health`, which stays reachable so
      container health checks and monitoring keep working (test for both).
- [ ] Table tests for addresses of every class incl. IPv4-mapped IPv6 and malformed RemoteAddr.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/http/... ./internal/platform/...`
      + coverage gate.

## Notes
- Docker bridge networking makes host-side clients appear from the bridge gateway (private), so
  the guard does not break NAS setups; a reverse proxy on the LAN also appears private, which is
  the documented trade-off.
- Wiring (`server.Options`) happens in M2-32.
- If OD-10 is rejected, drop this task and say so in `M2-00-overview.md`.
