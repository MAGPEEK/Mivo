# M1-03 Go quality gates: golangci-lint, depguard layer rules, architecture test, coverage
Owner: mivo-backend
Depends on: M1-02
Files/areas: `.golangci.yml`, `.testcoverage.yml`, `go.mod`, `go.sum` (tool directives),
`internal/archtest/**`, lint fixes in Go files created by M1-02

## Goal
Every Go rule from `CLAUDE.md` is enforced by a command: lint (incl. layer rules, size and
complexity limits), an architecture test inside `go test`, coverage thresholds and a
vulnerability scan.

## Acceptance criteria
- [ ] `.golangci.yml` uses the v2 format. `golangci-lint config verify` passes.
- [ ] Enabled linters and settings match ADR 0019: funlen 50 lines, gocognit 15, revive
      `argument-limit` 4 and exported doc comments, forbidigo for `panic` outside `cmd/` and
      for package-level `slog.Info|Debug|Warn|Error|Default|SetDefault` in `internal/`.
      Formatters are gofumpt and goimports with local prefix `github.com/magpeek/mivo`.
- [ ] depguard rules implement the layer matrix in ADR 0019 (domain, app, platform,
      adapters).
- [ ] Generated files (`Code generated … DO NOT EDIT`) are excluded. `_test.go` files are
      exempt from funlen and mnd only.
- [ ] `golangci-lint run` and `golangci-lint fmt --diff` are clean on the repository.
- [ ] `internal/archtest` contains a test that parses every non-test `.go` file under
      `internal/` with `go/parser` (imports only) and fails on any matrix violation. Its
      checking function is unit-tested with in-memory fixtures (one allowed and one forbidden
      case per layer). A test asserting the real tree is clean passes:
      `go test ./internal/archtest/...`.
- [ ] `go-test-coverage` and `govulncheck` are pinned with `go get -tool`.
      `go test -coverprofile=coverage.out ./...` followed by
      `go tool go-test-coverage --config=.testcoverage.yml` enforces total ≥ 70 % and
      `internal/domain` ≥ 90 %. Generated packages (`apigen`, `sqlcgen`) and
      `internal/archtest` are excluded.
- [ ] `go tool govulncheck ./...` runs (needs network access; report the result).
- [ ] The REPORT shows a temporary violation (for example `import "database/sql"` in a domain
      file) producing a depguard error and an archtest failure, and confirms it was
      reverted.
- [ ] Gates: all of the above pass.

## Notes
- golangci-lint itself is installed as a binary (being installed on the dev machine), not as
  a go tool. Record the version used in the REPORT so M1-22 can document it.
- The archtest duplicates depguard on purpose. It works without golangci-lint and gives the
  tester a plain `go test` signal.
- At this point the tree is mostly `doc.go` files. Thresholds must still pass. Do not lower
  them later; add tests instead.
- Prefer `errorlint` over `wrapcheck` (too noisy). If a listed linter does not exist in the
  installed v2 release, say so in the REPORT rather than silently dropping it.
