# M2-31 test report (LAN-only guard)

Status: **PASS** (round 3)
Base: clean detached worktree of `265e4bf` plus exactly the 16 files of the M2-31 change set
(HEAD in the main tree moved to `241b227` during the run; the worktree was not affected).
Worktree removed afterwards, `git worktree prune` run.

## History
- Round 3 (this report): adds on-link IPv6 prefixes (provider, 30 s cache, single refresher, drop after
  4 failed refreshes), fixed 403 `detail`, `DisableGeneralOptionsHandler`, docs. PASS.
- Rounds 1-2: earlier reports, overwritten by this one.

## Gates (all in the worktree)

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...` | pass | exit 0 |
| `go vet ./...` | pass | no output |
| `go test "-coverprofile=coverage.out" ./...` (Windows) | pass | all 38 packages `ok`, exit 0 |
| `go-test-coverage --config=.testcoverage.yml` | pass | `Total coverage threshold (70%) satisfied: PASS`, total 97.0 % (4753/4898) |
| `go tool cover -func` per new file | pass | `languard.go`, `onlinkprefixes.go` 100 % in every function; `interfaceprefixes.go` 100 % except `InterfacePrefixes` 83.3 % (the `net.Interfaces`/`Addrs` error branches); `config/settings.go` `applyLANOnly` 100 % |
| `internal/domain/...` coverage >= 90 % | pass | 100 % in every domain package |
| `golangci-lint run --allow-parallel-runners` | pass | `0 issues.` |
| `golangci-lint fmt --diff` | pass | no diff |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | pass | exit 0 both |
| archtest (`./internal/archtest`) | pass | `ok`; also inside the full run |
| Domain layer rule | pass | `go list -deps ./internal/domain/...` has no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` |
| Linux race, Docker: `go test -race ./...` | pass | 33 `ok`, exit 0 (`server` 1.8 s) |
| Linux race, Docker: `go test -race -count=20 ./internal/adapters/http/server/...` | pass | `ok ... 15.381s`, no failure in 20 rounds |
| `TestBodyReadDeadlineSparesBodilessStreams` (load-sensitive) | no flake seen | passed in the full race run and in all 20 rounds |
| `pnpm -C web install --frozen-lockfile --offline` | pass | `Done in 15.3s`, exit 0 |
| `pnpm -C web e2e` (guard active in binary, Chromium over loopback) | pass | `10 passed (9.4s)`, exit 0 |
| Web lint/test | not run | red on HEAD for the unrelated M2-22/M2-23 reason, per instruction |

Note: no cgo toolchain on this Windows machine, so `-race` ran only in Docker (golang:1.27.3).

## Probes (binary and httptest-level)

Method. The first group uses the real `server.New`/`Run` chain over a real TCP socket with a
listener wrapper that reports a chosen `RemoteAddr` (the same value net/http gives handlers), a
stub API handler and a fake `InterfacePrefixProvider`. The probe program lived only in the
worktree and is deleted. The second group runs the real binary built from the worktree
(`go build ./cmd/mivo`).

Fake peer, fake provider (Teredo `2001:0:5ef5:79fb::/64`, 6to4 `2002:cb00:7107::/48`, NAT64
`64:ff9b::/96`, valid `2001:db8:1:2::/64`):

| Peer / request | Result |
|---|---|
| real loopback with `X-Forwarded-For`/`X-Real-IP`/`Forwarded: 8.8.8.8` | 200 (headers irrelevant) |
| `203.0.113.7` GET `/api/v1/libraries` with and without spoofed `X-Forwarded-For: 192.168.1.5`, `X-Real-IP: 10.0.0.5`, `Forwarded: for=192.168.1.5` | 403 both times |
| `192.168.178.5` with `X-Forwarded-For: 8.8.8.8` | 200 (LAN socket peer wins) |
| `203.0.113.7` GET and HEAD `/api/v1/health`, GET `/api/v1/health?x=1` | 200 |
| `203.0.113.7` POST, OPTIONS `/api/v1/health`; GET `/api/v1/health/`, `/api/v1/healthz`, `/api%2Fv1%2Fhealth`, `/api/v1/heal%74h`, `/` | 403 |
| 403 body | `{"code":"forbidden","detail":"Requests from outside the local network are refused (server.lan_only).","status":403,"title":"Forbidden","type":"about:blank"}`, `Content-Type: application/problem+json`, `X-Request-Id`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` present |
| `[::ffff:192.168.1.5]` / `[::ffff:8.8.8.8]` / `100.64.1.1` | 200 / 403 / 200 |
| Teredo `[2001:0:5ef5:79fb:0:59d:a0e5:ba1]`, 6to4 `[2002:cb00:7107:1::1]`, NAT64 `[64:ff9b::cb00:7109]` although their prefixes are reported by the provider | 403 all three |
| `[2001:db8:1:2::77]` (inside valid reported /64) / `[2001:db8:1:3::77]` (next /64) | 200 / 403 |
| `OPTIONS *` from `203.0.113.7` | 403 with the problem body (goes through the chain; net/http's own answer is disabled); from `192.168.178.5` it reaches the mux (400) with security headers and request id |
| Debug log for each refusal | `level=DEBUG msg="request from a public address refused" method=... path=... request_id=... remote_address=...`; `path=*` for `OPTIONS *` |
| `AllowPublicClients: true`, public peer | 200; listening line `lan_only=false` |
| Default | listening line `... address=127.0.0.1:54907 lan_only=true` |

This machine (Windows 11, interfaces up and not loopback, IPv6):
- `vEthernet (Internal Bond)`: `2003:e3:e703:ab00:fcb0:ecb1:3504:f41c/64`, `fd8c:76aa:6082:0:4ac2:9fa0:726d:ecfb/64`,
  temporary `2003:e3:e703:ab00:9092:e398:180b:aae1/128`, `fd8c:...:180b:aae1/128`, `fe80::1220:1a6e:57f2:48ab/64`
- `vEthernet (ACS)`, `(Default Switch)`, `(WSL (Hyper-V firewall))`: link-local `/64` only

A global IPv6 address exists, so the real path was exercised:
- Server on `[::]:0` with the default (system) provider, client connected **from** the global address
  to itself (`remote [2003:e3:e703:ab00:fcb0:ecb1:3504:f41c]`): **200**; same for the temporary address: **200**.
- Control with an empty provider and the same two connections: **403** `forbidden`. So the pass comes from
  the interface prefix, not from another rule.
- Real binary (`MIVO_SERVER_PORT=18461`, default config) with curl and `X-Forwarded-For: 8.8.8.8`:
  `127.0.0.1`, `[::1]`, `192.168.178.22` and `[2003:...:f41c]` all reach the handler (501 = route not
  implemented in this HEAD, not a refusal); `[2003:...:aae1]` GET `/api/v1/health` 200; `OPTIONS *` 400 with
  security headers and request id; no "refused" line in the log; `http server listening address=[::]:18461 lan_only=true`.

Configuration (`server.lan_only`, env `MIVO_SERVER_LAN_ONLY`, real binary):
- `true`, `TRUE`, `False` and YAML `false`, `FALSE` are accepted.
- `yes`, `1`, `0`, `on`, empty value: startup error
  `mivo: load configuration: environment variable MIVO_SERVER_LAN_ONLY: invalid configuration value: "yes" is not one of true, false`, exit 1.
- YAML `yes`, `'1'`, `off`, empty, `null`: startup error naming the file, line and key `server.lan_only`.

## Acceptance criteria

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | Middleware refuses non-loopback/RFC 1918/RFC 6598/link-local/IPv6 ULA-link-local-loopback peers with 403 `forbidden` and a debug log; `X-Forwarded-For` ignored | met | probes above; `TestLANGuard`, `TestLANGuardTrustsOnlyTheSocketPeer`, `TestFixedLANRanges` pass; debug line verified |
| 2 | Config key `server.lan_only` (bool, default true, env `<PREFIX>_SERVER_LAN_ONLY`); startup log says the mode | met at server and config level | config tests and probes above; `lan_only=true|false` on the listening line (`TestRunLogsTheClientAccessMode`). See finding 1 for the binary |
| 3 | Guard covers every route except `GET /api/v1/health` (tests for both) | met | health GET/HEAD pass, POST/OPTIONS/variants refused; all other routes and `/` refused; `TestLANGuard` cases |
| 4 | Table tests for every address class incl. IPv4-mapped IPv6 and malformed RemoteAddr | met | `TestFixedLANRanges` (50 rows incl. mapped, hex-mapped, NAT64, 6to4, empty, garbage, no port, zone) and `TestTrustedOnLinkPrefixes` (25 rows) |
| 5 | Gates: lint, layer rules, race tests of `./internal/adapters/http/...` and `./internal/platform/...`, coverage gate | met | table above |

Round-2/3 additions: single refresher and first-load-only blocking (`TestOnLinkPrefixesRefreshDoesNotBlockOtherCallers`,
`...FirstLoadRunsOnce`, `...ConcurrentCallers`, all race-clean in 20 rounds), drop after 4 failed refreshes
with at most two warnings per streak (`...DropStalePrefixesAfterRepeatedFailures`, `...FailClosedWithoutKnownPrefixes`),
Teredo/6to4/NAT64/ULA/IPv4/short prefixes untrusted (`TestTrustedOnLinkPrefixes`, probe): all pass.

## Documentation check

Verified against behaviour: key name, env name, default, case-insensitive `true`/`false`, rejection of
`yes`, `no`, `on`, `off`, `1`, `0`, empty (ADR 0018 table); fixed detail text; `OPTIONS *` goes through the guard;
`lan_only=` on the listening line; /48 minimum, 2000::/3, Teredo/6to4 exclusions; 30 s refresh; "about two
minutes" (4 x 30 s); one warning at streak start and one at drop; /128 DHCPv6 addresses yield no neighbour
prefix (seen here: the temporary address is a /128); IPv4 interface subnets never trusted; `100.64.0.0/10`
costs; XFF ignored. `packaging/docker/README.md` is consistent with the ADR.

Not verifiable on this machine (Windows, Docker Desktop, no Linux Docker Engine with userland proxy): the Docker
bridge claims (IPv6 via docker-proxy appears as `172.17.0.1`, real client IPv4 address preserved on Linux). They
match documented Docker Engine behaviour and are consistent with the Docker Desktop bullet.

## Findings (none blocking)

1. The config value is parsed and validated but not yet used by the binary. `cmd/mivo` does not pass
   `Options.AllowPublicClients`, so with `MIVO_SERVER_LAN_ONLY=false` (or YAML `false`) the real binary still logs
   `lan_only=true` and still refuses public peers. This is the stated scope split (task Notes: "Wiring happens in
   M2-32"; M2-32 has the explicit item and test). The ADR 0018 note and `packaging/docker/README.md` ("`false` turns the guard off",
   "Set `MIVO_SERVER_LAN_ONLY=false` only when...") describe the intended behaviour as if shipped; they are correct only after M2-32.
   Do not release between M2-31 and M2-32 with these docs.
2. Wording: ADR 0018 says the last prefixes stay trusted "for at most 4 consecutive failed refreshes". The code and test keep them
   through 3 failed refreshes and drop them on the 4th. The "about two minutes" figure is right.
3. Windows `-race` is impossible without a C toolchain; Docker covers it.
4. The task file's acceptance checkboxes are still unticked (orchestrator's decision).

## Outcome

All gates and all five acceptance criteria are met; no threshold is missed.

REPORT
status: PASS
task: M2-31
files: docs/reports/M2-31-test.md
tests: go build/vet ok; go test -coverprofile ./... ok (go-test-coverage total 97.0 %, threshold 70 %, domain 100 %); golangci-lint run 0 issues; golangci-lint fmt --diff clean; go mod tidy -diff clean (root and tools); archtest ok; Docker go test -race ./... ok (33 packages); Docker go test -race -count=20 ./internal/adapters/http/server/... ok (15.4 s, no flake incl. TestBodyReadDeadlineSparesBodilessStreams); pnpm install --frozen-lockfile --offline ok; pnpm -C web e2e 10 passed; probes ok (loopback, spoofed headers ignored, health exemption exact, 403 body/headers/log, Teredo/6to4/NAT64 untrusted, OPTIONS * through chain, real global IPv6 2003:e3:e703:ab00::/64 passes and 403 with empty provider, lan_only config validation, lan_only= on listening line)
findings:
- config key is validated but not yet wired in cmd/mivo, so MIVO_SERVER_LAN_ONLY=false still logs lan_only=true until M2-32; ADR 0018 / Docker README describe the post-M2-32 behaviour (planned, in M2-32)
- ADR wording "at most 4 consecutive failed refreshes" is off by one (dropped on the 4th); minor
- Docker bridge/IPv6 claims not verifiable on Docker Desktop; consistent with documented Engine behaviour
next: proceed to mivo-reviewer / commit; make sure M2-32 wires AllowPublicClients and the interface-prefix provider before any release
