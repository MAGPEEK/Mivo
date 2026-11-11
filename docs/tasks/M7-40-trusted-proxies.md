# M7-40 Trusted reverse proxies
Owner: mivo-backend
Depends on: M7-20, M7-21 (both merged); must merge before M7-25
Files/areas: `internal/platform/config/` (new key, own file like `browse.go`, plus its line in `settings.go`/`config.go`), `internal/adapters/http/server/` (new resolver file, `languard.go`, `manageguard.go`, `auth.go`, `sameorigin.go`, one `Options` field), `internal/adapters/http/restapi/auth.go` (the `secure` flag only), their tests, the settings table of ADR 0018 if it lists keys. Not `cmd/mivo` (M7-25 wires the option).

## Goal
An owner who runs the server behind a TLS-terminating reverse proxy lists the proxy in
`server.trusted_proxies`. Then the session cookie is `Secure`, sign-in and other unsafe
requests pass the same-origin rule over HTTPS, internet clients are classified `remote` (no
first-run setup, no administration), each client has its own sign-in throttle bucket and the
device list shows the right address class. Without the setting nothing changes and no
forwarding header is ever believed (ADR 0032 §3 "Trusted reverse proxies", §5, §9).

## Acceptance criteria
- [ ] Config: `server.trusted_proxies` accepts a list of CIDR prefixes and single addresses
      (IPv4, IPv6) from the config file and the environment like the other list keys; default
      empty. An entry that does not parse, a duplicate, and a prefix of length 0 fail start-up
      with an error wrapping `config.ErrInvalid` that names the key and the entry. Entries are
      stored masked; IPv4-mapped IPv6 entries are unmapped.
- [ ] One resolver in `adapters/http/server` computes the client address and scheme once per
      request; the LAN guard, the manage guard, `authGuard.peer` (throttle address, device
      address class), `originMatches` and the cookie `secure` flag in `restapi` use only its
      result. `request.RemoteAddr` and `request.TLS` are read in no other place of
      `internal/adapters/http` (check:
      `rg "RemoteAddr|\.TLS\b" internal/adapters/http --glob "!*_test.go"` lists only the
      resolver file).
- [ ] Empty list: behaviour is byte-for-byte as today; requests carrying `X-Forwarded-For`,
      `X-Forwarded-Proto`, `X-Forwarded-Host`, `Forwarded` or `X-Real-IP` resolve to the
      socket peer and the socket scheme (test).
- [ ] Spoofing: with a non-empty list, a request whose socket peer is **outside** the list
      and that sends `X-Forwarded-For: 192.168.1.10` and `X-Forwarded-Proto: https` resolves
      to its socket address and `http`; a public peer doing so is still refused by setup and
      the manage guard and gets no `Secure` cookie (test through the handler chain).
- [ ] Trusted peer, table tests for the address:
      single hop; several hops in one header (`client, proxy1` with proxy1 trusted → client);
      several header lines (joined in order); a client-supplied left prefix
      (`10.0.0.1, 203.0.113.7` from the trusted peer → `203.0.113.7`, class `remote`); all
      entries trusted → left-most; entry with port (`203.0.113.7:4711`), bracketed IPv6 with
      and without port (`[2001:db8::1]:4711`), bare IPv6, IPv4-mapped IPv6
      (`::ffff:192.168.1.10` → `192.168.1.10`, class `lan`); surrounding spaces.
- [ ] Fail closed, table tests: trusted peer without `X-Forwarded-For`, with an empty value,
      with `unknown`, an obfuscated identifier (`_hidden`), a zone id, or garbage before the
      client is found → class `remote`, address = the proxy's socket address; setup and
      manage requests are refused.
- [ ] Scheme, table tests: trusted peer with `https`, `HTTPS`, `http`, no header, two values
      (the last one decides), an unknown value (→ `http`); untrusted peer with `https` on a
      plain socket (→ `http`). With `https` resolved, sign-in sets the cookie with `Secure`,
      sign-out clears it with `Secure`, and an unsafe request with
      `Origin: https://<Host>` passes while `Origin: http://<Host>` fails.
- [ ] `X-Forwarded-Host` never changes the same-origin host (test: trusted peer, differing
      `X-Forwarded-Host`, the `Host` header decides).
- [ ] Integration test in `adapters/http/server`: a server configured with the loopback
      address as trusted proxy, a request with a public `X-Forwarded-For` gets `403` on
      `POST /api/v1/setup` and on a manage route; a LAN `X-Forwarded-For` passes the guard.
- [ ] Log lines of the guards carry the resolved client address and, when it differs, the
      proxy address (no header values verbatim; truncated like today).
- [ ] Tests: `go test ./internal/platform/config/ -run TrustedProx` and
      `go test ./internal/adapters/http/... -run 'Proxy|ClientOrigin|SameOrigin|Guard|Cookie'`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Algorithm and decisions are fixed in ADR 0032 §3; do not widen them (no `Forwarded`, no
  `X-Real-IP`, no trust by default, no "trust private ranges" shortcut).
- `server` imports `restapi`, so the resolved value cannot live in `server` if `restapi`
  reads it. Either keep passing it the way `clientRequest` is filled today (the middleware
  in `restapi/auth.go` receives a small function or value from `server`), or put the type
  and its context accessor in a tiny package both import. No globals.
- Classify the resolved address with the existing LAN classification (private ranges,
  on-link prefixes); do not write a second one.
- `server.Options` gets the parsed prefix list (`[]netip.Prefix`); its zero value means
  "trust nobody". M7-25 passes `config.Server.TrustedProxies` in `cmd/mivo` and tests it.
- Comments in `languard.go`/`manageguard.go` that say forwarding headers are ignored until
  M7b must be corrected.
- Out of scope: masking IPv6 throttle addresses to /64 (backlog "m7-15 review 2 minors",
  M7b); the user-facing guide text with proxy examples (M7-39); a Host allow-list (m3-19k).
- Pitfall: `http.Header.Values`, not `Get`, or a second header line is lost. Bound the
  number of entries read (named constant) so a huge header cannot burn CPU; more entries
  than the bound is a fail-closed case.
