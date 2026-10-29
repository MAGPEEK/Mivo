# M1-06 System domain and use cases: server identity, health, system info
Owner: mivo-backend
Depends on: M1-02, M1-03
Files/areas: `internal/domain/errors.go` (+ test), `internal/domain/system/**`,
`internal/app/system/**`, `internal/adapters/uuidgen/**`, `go.mod`, `go.sum`

## Goal
The walking skeleton's business core: the server has a stable identity (UUID created once),
can report its health (database reachable or not) and can describe itself (product name,
version, server ID). All of this is pure logic plus use cases behind ports, with no I/O
in domain or app.

## Acceptance criteria
- [ ] `internal/domain` defines sentinel errors `ErrNotFound`, `ErrConflict`, `ErrInvalid` and
      `ErrForbidden` with doc comments.
- [ ] `internal/domain/system` defines:
      - `ServerID` value object with `ParseServerID(string) (ServerID, error)` that accepts
        only canonical lowercase UUID text (8-4-4-4-12) and wraps `ErrInvalid` otherwise
      - `Identity{ID ServerID; CreatedAt time.Time}`
      - `HealthStatus` (`ok`, `unavailable`) and `Health{Status; Checks []HealthCheck}` with
        `HealthCheck{Name, Status, Detail}`
      - `Info{ProductName, Version, Commit, BuildTime, ServerID, APIVersion}`
- [ ] `internal/app/system` declares the ports it needs (each 1–2 methods):
      `IdentityStore` (load and save), `DatabaseChecker` (`Ping(ctx) error`) and
      `IDGenerator` (`NewID() (string, error)`). `Service` is built from a dependencies
      struct (ports, clock `func() time.Time`, build info value, product name).
- [ ] `EnsureIdentity(ctx)` loads the identity. On `ErrNotFound` it generates, validates and
      saves a new one. It is idempotent (second call returns the same ID, saves once). Other
      store errors are returned wrapped with context.
- [ ] `Health(ctx)` returns `ok` when ping succeeds. It returns `unavailable` with a
      `database` check and the error detail when ping fails or exceeds a named timeout
      constant (2 s). The deadline is honoured through the context.
- [ ] `Info(ctx)` returns build values, product name, API version `"1"` and the persisted
      server ID.
- [ ] `internal/adapters/uuidgen` implements `IDGenerator` with random UUIDs in canonical
      lowercase form.
- [ ] Tests: table-driven domain tests without mocks (`internal/domain/...` ≥ 90 %). App tests
      use hand-written in-memory fakes (no mock framework) and cover every branch above. One
      uuidgen test checks format and uniqueness over a few calls.
- [ ] Gates: `golangci-lint run` (incl. depguard), `go test ./...`, archtest and coverage
      check pass.

## Notes
- `app/system` must not import `platform/buildinfo` or `platform/brand` (ADR 0003). Define a
  small `BuildInfo` struct in `app/system`. `cmd` maps the platform values into it (M1-10).
- Use `github.com/google/uuid` (BSD-3-Clause) unless the Go version in use ships a standard
  library UUID package; state the choice in the REPORT.
- M1-08 implements `IdentityStore` and `DatabaseChecker` in the sqlite adapter. Keep method
  names and signatures stable once this task is merged, because M1-08 and M1-09 depend on
  them.
