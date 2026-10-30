# M2-18 test report: app/library catalog use cases

Task file `docs/tasks/M2-18-app-library-catalog.md` including the "Orchestrator note (from M2-10)" with its additional
Files/areas and all appended notes.
Files under test: `internal/app/library/**` (ports.go, service.go, libraries.go, items.go, availability.go and tests),
`internal/domain/library/summary.go`, `internal/adapters/sqlite/library.go` + `library_test.go` (return types and
`SyncResult`), `docs/adr/0022-media-library-model-and-scanning.md` ("Notes (M2-18)").
Round 1. Result: **PASS**.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...`, `go vet ./...` | PASS | exit 0 |
| `go test -race -count=1 -coverprofile` for `./internal/adapters/sqlite/... ./internal/app/library/... ./internal/domain/library/... ./internal/app/scan/... ./internal/app/playback/...` | PASS | `app/library 100.0%`, `domain/library 100.0%`, `sqlite 94.8%`, `app/scan 97.9%`, `app/playback 98.6%`; compile-time port assertions hold (`ItemReader`, `FileSource`, `scan.CatalogWriter`; the `scan.LibrarySource` consumer compiles against `library.Summary`) |
| `go test -race -count=30 ./internal/app/library/...` | PASS | `ok .../app/library 3.845s` (no flake in the availability and join tests) |
| Coverage gate `go-test-coverage --config=.testcoverage.yml` (full run profile in the scratchpad) | PASS | total 96.9 % (4258/4394) >= 70; every `internal/domain/...` package >= 90 (`domain/library` 100 %); `app/library` 100 % |
| `golangci-lint run --allow-parallel-runners`, `golangci-lint fmt --diff` | PASS | `0 issues.`, fmt exit 0 |
| sqlc drift (`sqlc/sqlc:1.31.1 diff`) | PASS | exit 0, empty output (`library.go` queries unchanged) |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | PASS | exit 0 |
| archtest | PASS | `ok .../internal/archtest` |
| Layer rule (independent) | PASS | `go list` imports of `internal/app/library`: only `internal/domain`, `internal/domain/library`, `internal/domain/media` (plus stdlib); tests add no adapter; `internal/domain/...` deps list no `database/sql`, `net/http`, `os/exec`, adapters or app |
| Linux run (`golang:1.27.3`, repo `:ro`) `go test -race -count=1 ./internal/adapters/sqlite/...` | PASS | `ok ... 17.123s` (covers `library_test.go` with the new types) |
| Stray files / TODO | PASS | no `TODO`/`FIXME` in `internal/app/library`, `internal/domain/library`, `internal/adapters/sqlite`; nothing from this run left in the repo |

## Acceptance criteria and notes

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Ports (1-3 methods each): library store (`SyncLibraries`, `ListLibraries`, `GetLibrary`), item reader (`ListItems`, `GetItem`), ID generator | met | `ports.go`: `Store` 3, `ItemReader` 2, `IDGenerator` 1, `RootChecker` 1 methods; consumer-declared, no adapter import |
| 2 | `SyncLibraries(ctx, []Definition)` validates every definition (`ErrInvalid` names the index), delegates, returns the synced libraries | met | `libraries.go:25`; probe: `sync libraries: definition 1: validate library definition: name is empty: invalid`; catalog untouched |
| 3 | `ListLibraries`, `GetLibrary`, `ListItems` (limit via `NormalizeLimit`, unknown library `ErrNotFound`), `GetItem` returning `ItemDetail{Item, Files}` | met | `libraries.go`, `items.go`; probe against the real SQLite adapter: limit 501 and cursor `%%%` give `ErrInvalid`, unknown library `ErrNotFound` |
| 4 | Availability via `RootAvailable(ctx, root) bool` with a short timeout so a hung NAS mount cannot block the list | met | `availability.go`, `DefaultRootCheckTimeout = 1s`; probe below |
| 5 | `NewService` rejects nil dependencies with `ErrInvalid` | met | `service.go:64` (also a negative timeout); `service_test.go` |
| 6 | Tests with in-memory fakes (no mock framework); coverage >= 90 % | met | `fakes_test.go` hand-written fakes; `app/library` 100.0 % |
| 7 | Gates: lint + layer rules + `go test -race ./internal/app/...` + coverage gate | met | see gates |
| N1 | `library.Summary{Library, ItemCount}` in the domain, port declared with it, sqlite return types switched, `sqlite.LibrarySummary` removed | met | `domain/library/summary.go`; `adapters/sqlite/library.go:71,82` return `library.Summary`; `grep LibrarySummary` has no production match |
| N2 | Removed libraries returned by `SyncLibraries` (name, root, item count) and logged at warn; tested | met | `library.SyncResult{Libraries, Removed}`; probe below |
| N3 | `RootAvailable` not implemented in `libraryfs` (M2-12 in review); port declared here, fake used | met | `git status` shows nothing under `internal/adapters/fs/libraryfs` from this task; `RootChecker` in `ports.go` |
| N4 | ADR 0022 Notes (M2-18) | met | `docs/adr/0022-...md:176-193` documents sync validation and warn log, 1 s parallel availability, join-a-running-check trade-off, error contract of the catalog reads |

## Independent probes (scratch module in the scratchpad, real SQLite in temp dirs, repo untouched)

All run with `-race`; file `scratchpad/qa-m215/probe/probe_test.go`.

| Probe | Result | Evidence |
|---|---|---|
| `SyncLibraries` with a removed library logs a warn with the item count | PASS | 3 items in "Shows" (root `C:/s`), then syncing only "Movies": exactly 1 record, `level=WARN`, `item_count=3 library_id=<id> name=Shows root=C:/s`; the items are gone by cascade (0 rows) |
| Duplicate roots rejected with the index | PASS | `sync libraries: definition 2: root "C:/m" is already used by definition 0: invalid`; libraries table, items, files and streams unchanged, no warn record. The adapter rejects the same input too (`ErrInvalid`) |
| Availability check with a hanging checker | PASS | timeout 150 ms: first `ListLibraries` returned in 157.6 ms; hung root `A` unavailable, other root `B` available; 101 further `ListLibraries`/`GetLibrary` calls while hung: the hung root was checked exactly once (joined), goroutines base 5, during 6; after releasing the checker the root is available again and goroutines are back at 5 (no leak); a cancelled context returns an error instead of "all unavailable" |
| Undecodable stored row through the use case: no 400 path | PASS | corrupt item kind / container / frame rate / relative path (ListItems, GetItem) and corrupt library kind (ListLibraries, GetLibrary, ListItems): every error is plain, `errors.Is(ErrInvalid)` and `errors.Is(ErrNotFound)` both false (the message text ends in "invalid", but the chain is not wrapped); the library-kind case passes through `storedDataError` ("unreadable stored library data: ...") |
| Real bad client values still map | PASS | cursor `%%%` and limit 501 through the service: `ErrInvalid`; unknown library: `ErrNotFound` |

## Findings (none blocking)

- `internal/adapters/sqlite/library.go:293-299` still wraps `domain.ErrInvalid` for a stored library kind the domain does
  not know (adapter-level probe: `wraps ErrInvalid=true`). `app/library` neutralises it (`storedDataError`,
  `libraries.go:117`), so every path through the use case is a plain error, and the doc comment says so. But
  `docs/tasks/M2-26-restapi-library-endpoints.md:37-38` says "stores no longer wrap ErrInvalid for undecodable stored
  rows": that holds for the catalog reader and writer, not for `LibraryRepository`. The HTTP adapter (M2-26) must call
  the libraries only through `app/library.Service`; wired directly to the repository, a corrupt library row would
  answer 400. Optional hardening: make `summaryFromRow` use the same non-`ErrInvalid` error as the catalog.
- Trade-off as documented: a root check that never returns keeps its root unavailable (one blocked goroutine per root at
  most). Verified by the probe above; not a defect.

## History

- Round 1: PASS (this report).
