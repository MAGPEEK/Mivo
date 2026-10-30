# M1-03b Harden layer gates and isolate tool dependencies
Owner: mivo-backend
Depends on: M1-03
Files/areas: `.golangci.yml`, `internal/archtest/**`, `go.mod`, `go.sum`, `tools/go.mod`, `tools/go.sum`, `docs/adr/0019-quality-gates.md` (append a short note only)

## Goal
Close the gaps found in the M1-03 review so depguard and the architecture test enforce the
same "no I/O in domain" rule, and keep tool-only dependencies out of the production module graph.

## Acceptance criteria
- [ ] `internal/domain` may not import any `os/...` package (`os`, `os/exec`, `os/signal`, `os/user`): archtest uses a prefix match; depguard has explicit denies so the `$gostd` allow cannot win. `layers_test.go` lists `os/signal` as forbidden.
- [ ] Temporary violation demos (removed afterwards) for `net/http/httptest`, `database/sql/driver` and `os/signal` in a domain file each fail **both** depguard and archtest. Outputs quoted in the REPORT.
- [ ] Tool dependencies (go-test-coverage, govulncheck, gofumpt if used as tool) live in a separate `tools/go.mod`, invoked with `go tool -modfile=tools/go.mod …`. The main `go.mod` no longer contains AWS SDK / go-github / freetype or other tool-only requirements; `go mod tidy -diff` is clean for both modules.
- [ ] `internal/archtest` package comment documents the intended differences to depguard (test files, subpackages).
- [ ] `TestCheckTreeFindsViolationsAndSkipsTestsAndTestdata` (or a new test) covers skipping `_x/` and `.x/` directories.
- [ ] ADR 0019 gets a short "Notes" addition: funlen `ignore-comments: true`, tool modfile, archtest vs depguard scope.
- [ ] `go.mod` pins `toolchain go1.27.3` (or newer patch) so the patched toolchain is downloaded automatically (winget only offers 1.27.0); `go version` inside the module reports it, and govulncheck no longer lists the go1.27.0 stdlib advisories.
- [ ] Gates: `golangci-lint run` 0 issues, `go test -race ./...` green, coverage gate PASS, `go tool -modfile=tools/go.mod govulncheck ./...` 0 vulnerabilities affecting code.

## Notes
- Review source: M1-03 reviewer findings (minor 1–4, nits 1 and 3).
- Update any command examples that mention `go tool go-test-coverage` / `go tool govulncheck` (M1-22 will collect them in CLAUDE.md).

## Deferred (from round-2 review, APPROVE)
- Deny `go/build` and `go/importer` in the domain (both gates); add file-reading functions of
  `debug/*`, `archive/zip.OpenReader`, `text|html/template.ParseFiles|ParseGlob`,
  `go/parser.ParseDir`, `crypto/x509.SystemCertPool` to the "domain purity" forbidigo rules,
  or list them as accepted gaps in ADR 0019.
- `depguard_test.go`: include `exec.ExitError.Stderr` when `go list std` fails; check `readErr`
  before the close error; document that the sync check is one-way and std-only.
- ADR 0019: state that archtest is the authoritative gate for cgo (`"C"`).
Pick these up the next time `internal/archtest` or `.golangci.yml` is touched.
