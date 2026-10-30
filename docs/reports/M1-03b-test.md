# M1-03b test report

Status: PASS (round 2)
Date: 2026-10-30
Scope: Go gates only (the uncommitted web/ files belong to M1-14 and were ignored).

## Round 2

Changes verified: `.golangci.yml` (exact `$` denies for the full domain/app stdlib trees, forbidigo for
`filepath` and `log` functions in `internal/domain`), `internal/archtest/{layers.go,layers_test.go,tree_test.go,
depguard.go,depguard_test.go}`, `tools/go.mod`, ADR 0017 and ADR 0019, plus the earlier M1-03b changes
(go.mod toolchain pin, deleted go.sum, tools/go.sum, `.testcoverage.yml`).

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Toolchain | `go version` | `go version go1.27.3 windows/amd64` |
| Vet | `go vet ./...` | exit 0 |
| Tests + race | `go test -race -count=1 -cover ./...` | all ok; cmd/mivo 87.5 %, archtest 95.4 %, buildinfo 96.9 % |
| Coverage gate | `go test "-coverprofile=coverage.out" ./...` + `go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"` | `Total coverage threshold (70%) satisfied: PASS`, total 92.9 % (52/56). `internal/domain` has no statements yet, so the 90 % override matches nothing (`Package coverage threshold (0%) satisfied`). |
| Lint | `golangci-lint run` | `0 issues.` |
| Format | `golangci-lint fmt --diff` | no output, exit 0 |
| Tidy main | `go mod tidy -diff` | exit 0, no diff |
| Tidy tools | `go -C tools mod tidy -diff` | exit 0, no diff |
| Vulnerabilities | `go tool "-modfile=tools/go.mod" govulncheck ./...` | `No vulnerabilities found.` |

### Violation demos in internal/domain

Temporary files `internal/domain/demo_{smtp,ioutil,tls,walk,log,join}.go` were created, linted and
tested, then deleted. Only `doc.go` remains in `internal/domain`; `coverage.out` was removed; no
demo files remain (`git status` shows no untracked files besides the task's own).

Import denies - depguard (`golangci-lint run`, depguard lines):

```
internal\domain\demo_ioutil.go:3:8: import 'io/ioutil' is not allowed from list 'domain': the domain is pure; file system access belongs to an adapter (depguard)
internal\domain\demo_smtp.go:3:8: import 'net/smtp' is not allowed from list 'domain': the domain is pure; networking belongs to an adapter (depguard)
internal\domain\demo_tls.go:3:8: import 'crypto/tls' is not allowed from list 'domain': the domain is pure; networking belongs to an adapter (depguard)
```

Import denies - archtest (`go test -count=1 ./internal/archtest/`):

```
--- FAIL: TestRepositoryFollowsLayerMatrix
    tree_test.go:25: layer violation: internal/domain/demo_ioutil.go imports "io/ioutil": internal/domain must not import it
    tree_test.go:25: layer violation: internal/domain/demo_smtp.go imports "net/smtp": internal/domain must not import it
    tree_test.go:25: layer violation: internal/domain/demo_tls.go imports "crypto/tls": internal/domain must not import it
```

Function denies - forbidigo (archtest is import based and correctly does not flag these):

```
internal\domain\demo_log.go:7:2: use of `log.Println` forbidden because "domain purity: return errors or values; logging belongs to the caller" (forbidigo)
internal\domain\demo_walk.go:7:9: use of `filepath.Walk` forbidden because "domain purity: file system access belongs to an adapter" (forbidigo)
```

Allowed cases:

- `filepath.Join("a", "b")` in `internal/domain/demo_join.go`: no depguard or forbidigo finding (only
  gofumpt/goimports noise from the throw-away file's line endings, as for every demo file).
- `filepath.WalkDir(".", nil)` in `internal/platform/demo_walkdir.go` (outside `internal/domain`): no
  forbidigo finding (only the same formatting noise). The exclusion `path-except: ^internal/domain/` works.

Side observation: the demo files also drew revive `blank-imports` and staticcheck `SA1019` (io/ioutil
deprecated) findings, which is expected for throw-away blank imports and not a defect.

### Mutation check of TestDepguardDeniesEveryStandardPackageOfTheMatrix

The two lines of the `net/smtp$` deny entry (`.golangci.yml:94-95`) were removed temporarily:

```
--- FAIL: TestDepguardDeniesEveryStandardPackageOfTheMatrix (0.27s)
    depguard_test.go:34: depguard rule "domain" in .golangci.yml lacks `- pkg: net/smtp$` (denied by archtest for internal/domain)
```

The file was restored from a copy taken before the mutation. SHA-256 before and after are identical
(`385A23AC63AEC8E7433844783DE9C50ED3E0C6D80B9B234F56AE1DFF248D1F61`), `git diff --stat -- .golangci.yml`
is unchanged (88 insertions, 9 deletions), and after the restore `golangci-lint run` gives `0 issues.`
and `go test ./internal/archtest/` is green.

### Acceptance criteria

1. `os/...` forbidden in the domain: met. archtest uses `prefix("os")` (layers.go:90); depguard has exact
   denies for `os`, `os/exec`, `os/signal`, `os/user` (.golangci.yml:64-71). layers_test.go lists
   `os/signal` as forbidden (:38, :61) and asserts the prefix match (:209).
2. Demos for `net/http/httptest`, `database/sql/driver`, `os/signal` fail both gates: met in round 1 (below).
   In round 2 the same mechanism was proven for `net/smtp`, `io/ioutil`, `crypto/tls`, and the
   `.golangci.yml` entries for the round-1 packages are unchanged (lines 68, 82, 102), and
   `TestDepguardDeniesEveryStandardPackageOfTheMatrix` now enforces completeness against `go list std`.
3. Tool dependencies isolated: met. go.mod has no `require` or `tool` block, go.sum is deleted, tools live
   in `tools/go.mod`/`tools/go.sum`, both `tidy -diff` runs are clean, tools run through `-modfile`.
4. archtest package comment documents the differences to depguard: met (layers.go:1-20: test files,
   subpackages, build constraints, denied stdlib trees).
5. Skipping of `_x/` and `.x/`: met (tree_test.go:38-39 create `internal/domain/media/_scratch/draft.go` and
   `internal/domain/.cache/cached.go` with a forbidden import; test green).
6. ADR 0019 "Notes": met (funlen `ignore-comments: true`, tool modfile, archtest vs depguard scope; ADR 0017
   now references `tools/go.mod`). `.golangci.yml:199` sets `ignore-comments: true`.
7. `toolchain go1.27.3` pin: met (go.mod, `go version` = go1.27.3, govulncheck clean).
8. Gates: met (lint 0 issues, race tests green, coverage gate PASS, govulncheck clean).

### Findings

None blocking. The "Package coverage threshold (0%)" line is expected while `internal/domain` has no code.

## Round 1 (history)

Status: PASS. Gates identical to round 2 (archtest coverage then 93.5 %). Demos with `os/signal`,
`net/http/httptest` and `database/sql/driver` in `internal/domain` were rejected by both depguard and
archtest:

```
internal\domain\demo_driver.go:3:8: import 'database/sql/driver' is not allowed from list 'domain': ... (depguard)
internal\domain\demo_httptest.go:3:8: import 'net/http/httptest' is not allowed from list 'domain': ... (depguard)
internal\domain\demo_signal.go:3:8: import 'os/signal' is not allowed from list 'domain': ... (depguard)
tree_test.go:33: layer violation: internal/domain/demo_driver.go imports "database/sql/driver": internal/domain must not import it
tree_test.go:33: layer violation: internal/domain/demo_httptest.go imports "net/http/httptest": internal/domain must not import it
tree_test.go:33: layer violation: internal/domain/demo_signal.go imports "os/signal": internal/domain must not import it
```

## History

- Round 1: PASS.
- Round 2: PASS.
