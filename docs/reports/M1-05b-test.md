# M1-05b test report

Status: PASS (round 1)
Scope: `./internal/platform/logging/...`

## Gates

| Gate | Command | Result |
|------|---------|--------|
| vet | `go vet ./internal/platform/logging/...` | exit 0 |
| test | `go test -race -count=1 -cover ./internal/platform/logging/...` | `ok ... coverage: 100.0% of statements` |
| coverage by function | `go tool cover -func` | every function 100.0 %, total 100.0 % |
| lint | `golangci-lint run ./internal/platform/logging/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/platform/logging/...` | exit 0, no diff |

Domain layer rule: not applicable (package is `internal/platform`; it imports no `internal/domain`).

## Acceptance criteria

1. Normalised key matching - MET. `normaliseKey` (redact.go:72) lower-cases and drops `_ - .`;
   `isSensitiveKey` (redact.go:59) uses suffix matching against `sensitiveKeySuffixes()`
   (redact.go:18, single place). All required words are covered, the derived ones through
   suffixes (accesstoken, refreshtoken, clientsecret, jwtsecret, setcookie, proxyauthorization).
   `TestNewRedactsSensitiveKeys` covers each in plain, snake, kebab, dotted and camel form.
   `tokenCount`, `token_count`, `passwordHint`, `secretary`, `publicKey` stay visible
   (`TestNewKeepsNonSensitiveAttributes`). The rule is documented in the package doc and on
   `isSensitiveKey`.
2. Table tests for api_key, x-api-key, access_token, Set-Cookie, LogValuer group under a
   sensitive key, `WithGroup("authorization")` - MET (`TestNewRedactsSensitiveKeys`,
   `TestNewRedactsSensitiveStructures` cases "LogValuer group under a sensitive key" and
   "logger WithGroup with a sensitive name"). `redactSensitive` also checks enclosing groups.
3. Error-message table (`"loud"`, `"xml"`, `""`) - MET (`TestNewRejectsInvalidOptions`,
   logging_test.go:117; each asserts `errors.Is` plus the quoted value, and nil logger).
4. Parallel subtests do not share a `bytes.Buffer` - MET. Every subtest declares its own
   buffer (`var output bytes.Buffer` inside `t.Run`; invalid-options test builds a writer per
   case). Run under `-race` is green.
5. Package doc states the limit (slog.Any whole values, LogValuer requirement, HTTP header
   allowlist in `adapters/http`) - MET (logging.go:17-22).
6. Gates: 100 % coverage, 0 lint issues, race green - MET (see above).

## Probe (outside the repo)

Probe source: `C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\probe\main.go`,
run via `go run -overlay` (virtual file, nothing was written into the repo; `git status`
shows only the four expected logging files modified). JSON and text handlers were both exercised.

Redacted, no secret value in output:
- `api_key`, `x-api-key`, `access_token`, `Set-Cookie`
- LogValuer group under `client_secret` (JSON: `{"value":"[REDACTED]","kind":"[REDACTED]"}`;
  text: `client_secret.value=[REDACTED] client_secret.kind=[REDACTED]`)
- `WithGroup("authorization")` leaves `scheme` and `param`
- `With("Authorization", ...)`, `slog.Group("headers", "X-Api-Key")`
- non-sensitive `ok=visible` stays visible

Expected, documented leak (not a failure): `slog.Any("hdr", http.Header{...})` logged as a
whole value writes `"hdr":{"Authorization":["TOPSECRET-VALUE-123"]}` (JSON) and
`hdr=map[Authorization:[...]]` (text). This is exactly the limitation stated in the package doc.

## Observations (non-blocking)

- Accepted over-redaction: `nextPageToken` and similar keys ending in `token` are redacted.
- Suffix matching leaves keys such as `tokenValue` or `passwordHash` visible. `passwordHash`
  and a raw `tokenValue` could hold sensitive material; consider whether a later task should
  add `hash`-style suffixes. Not required by M1-05b.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-05b
files: docs/reports/M1-05b-test.md
tests: go vet, go test -race -count=1 -cover (100.0 %), golangci-lint run (0 issues), golangci-lint fmt --diff (clean) on ./internal/platform/logging/...; overlay probe for api_key/x-api-key/access_token/Set-Cookie/LogValuer group/WithGroup("authorization") shows no leak in JSON or text
findings:
- all 6 acceptance criteria met
- only leak in probe is slog.Any(http.Header), the documented limitation
- non-blocking: passwordHash/tokenValue style keys remain visible by design of the suffix rule
next: proceed to mivo-reviewer
