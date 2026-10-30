# M2-10 test report: SQLite media catalog schema and library repository

Status: **PASS** (round 1)
Scope: `internal/adapters/sqlite/**` (migration 00002, `queries/libraries.sql`, `library.go`,
generated `sqlcgen`, the BACKLOG items "sqlite reader pool" and "sqlite nits"), ADR 0022 "Notes
(M2-10)". Wave 2 runs in parallel (ffmpeg, libraryfs, restapi, instancelock, web player-core
and others changed during the run). They are reported, not judged. At the end of the session
the whole repository built, vetted, linted and tested green, so nothing blocked the gates.
Windows 11, Go 1.27.3, golangci-lint 2.15.0, Docker 29.9.2.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/adapters/sqlite/...` (and `go vet ./...`) | PASS, no output |
| Tests, race | `go test -race -count=1 -coverprofile=... ./internal/adapters/sqlite/...` | PASS, `ok ... sqlite 3.831s coverage: 94.2% of statements` |
| Tests, flake | `go test -race -count=5 ./internal/adapters/sqlite/...` | PASS, `ok ... sqlite 7.801s` |
| Coverage gate | `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml` on the sqlite profile | PASS, total 94.2 % (194/206); `sqlcgen` is excluded by config as for all generated code |
| Coverage gate, whole repo | same, on `go test -race -count=1 ./...` | PASS, total 96.9 % (3083/3182); `Package coverage threshold (0%) satisfied`, `Total coverage threshold (70%) satisfied`; all `internal/domain/**` packages at 100 % |
| Lint | `golangci-lint run ./internal/adapters/sqlite/...` and unscoped `golangci-lint run` | PASS, `0 issues.` both |
| Format | `golangci-lint fmt --diff` (package and unscoped) | PASS, no diff |
| Layer rules | `go test ./internal/archtest/...` plus depguard inside lint | PASS; `go list -deps ./internal/domain/...` contains no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` |
| sqlc drift | `docker run --rm -v "${PWD}:/src" -w /src sqlc/sqlc:1.31.1 diff -f internal/adapters/sqlite/sqlc.yaml` | PASS, exit 0, empty output |
| Linux | `docker run golang:1.27.3`, repo mounted `:ro`, `go test -race -count=1 ./internal/adapters/sqlite/...` | PASS, `ok ... sqlite 2.211s`; the two UNC tests SKIP there, `TestReaderPool*` PASS |
| Windows UNC | `go test -v -run 'UNC\|ReaderPool\|Ping'` | PASS: `TestDataSourceNameKeepsAWindowsUNCPath`, `TestOpenWorksThroughAWindowsUNCPath` (real open through `\\localhost\C$`) ran and passed, not skipped |
| Whole repo | `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...` | PASS (first full-repo test run failed only because of my own mistyped `-coverprofile` argument, see "Notes"; rerun clean) |
| Docker image / media / web | n/a | not touched by this task, not run |
| TODO / FIXME | grep over the package and the diff | none |

No `go.mod` change by this task.

## Independent schema probes

The probes ran on a copy of the repository in the scratchpad (probe tests added there only; the
repository itself was not touched), against the real adapter: `sqlite.Open` + `Migrate` on a
temp DB, a second plain connection with `foreign_keys=ON`.

| Probe | Result |
|---|---|
| All four catalog tables `STRICT` | 4 of 4 (`pragma_table_list.strict = 1`) |
| Indexes | `items_library_sort_title (library_id, sort_title, id)`, `items_parent (parent_id)`, `media_files_item (item_id)`; `UNIQUE(library_id, relative_path)` serves `media_files.library_id` |
| FK cascade | library, item, file, 1 stream inserted; `DELETE FROM libraries` leaves 0 rows in `libraries`, `items`, `media_files`, `media_streams` |
| Unique root | duplicate `root_path` rejected |
| Unique (library, relative_path) | duplicate rejected |
| CHECK / type rejections (17 cases) | all rejected: item kind `show` and empty, library kind `music`, negative `size_bytes`, `duration_ms`, file `bit_rate`, stream `sample_rate`, `height`, empty `codec_tag`, empty `container`, orphan item, file and stream (FK), text in an integer column (STRICT), string in `size_bytes` |
| Accepted | `container 'unknown'` with `probe_error 'probe_failed'`, item kind `episode`, stream with `codec_tag 'hvc1'` and `frame_rate '24000/1001'` |
| Migration up from version 1 | a real M1 database (only `00001` applied by goose, one `server_identity` row), copied, then `Open` + `Migrate`: max version 2, identity row kept (1), all four tables exist, `pragma_foreign_key_check` = 0 rows |
| Second `Migrate` | no-op: no error, `goose_db_version` unchanged (2 rows with version > 0, max 2) |
| Up / down / up | covered by `TestMigrationsCanBeRolledBackAndReapplied` (tables and indexes gone after `DownTo(0)`, rebuilt by `Migrate`, version 2) |

## Mutation checks of the task's own tests

Each mutation was made in the scratchpad copy, the package tests were run, and the copy was
restored. "killed" means a test failed.

| Mutation | Result |
|---|---|
| SyncLibraries always updates unchanged libraries | killed: `TestSyncLibrariesTwiceWithTheSameInputChangesNothing`, `...UpdatesNameAndKindOfAKnownRoot` |
| Kind change does not update items | killed: `...UpdatesNameAndKindOfAKnownRoot`, `...RollsBackEverythingWhenAStatementFails/item_kind_update` |
| Removed roots are not deleted | killed: `...RemovesUnconfiguredRootsWithTheirCatalog`, `...WithoutDefinitionsRemovesEveryLibrary`, `.../library_delete` |
| No `SetMaxIdleConns(maxOpen)` | killed: `TestReaderPoolKeepsEveryOpenConnectionIdle` |
| `NumCPU` instead of `GOMAXPROCS(0)` (run with `GOMAXPROCS=2`) | killed: `TestReaderPoolKeepsEveryOpenConnectionIdle` |
| Zero `CreatedAt` accepted | killed: `TestSaveIdentityRejectsZeroValues/zero_created_at` |
| `media_streams` FK without `ON DELETE CASCADE` | killed: `TestMediaCatalogForeignKeysCascadeAndAreIndexed`, `TestSyncLibrariesRemovesUnconfiguredRootsWithTheirCatalog` |
| `items_library_sort_title` without `id` | killed: `TestMediaCatalogForeignKeysCascadeAndAreIndexed` |
| `codec_tag` CHECK removed | killed: `.../empty_codec_tag` |
| `STRICT` removed from `libraries` | killed: `TestMediaCatalogTablesAreStrict` |
| `GetLibrary` does not map `sql.ErrNoRows` | killed: `TestGetLibraryOfAnUnknownIDReturnsNotFound` |
| `Down` drops in forward order | **survived** (see observation 3) |
| Transaction is not rolled back on error | **survived** (see observation 1) |
| `rollBack` does not tolerate `sql.ErrTxDone` | **survived** (see observation 4) |
| `Ping` uses the driver ping instead of `pingQuery` | **survived** (see observation 2) |

## Acceptance criteria

1. Migration `00002`: MET. `STRICT`, cascade FKs, `UNIQUE(root_path)`, `UNIQUE(library_id,
   relative_path)`, the two named indexes, CHECKs for library kind, item kind
   (`movie|episode`, as `library.ItemKind`), stream kind and non-negative sizes; column lists
   match ADR 0022 plus the documented `codec_tag` (nullable, empty rejected) and the extra
   `items_parent` index, both recorded in ADR 0022 "Notes (M2-10)". `frame_rate` is TEXT with
   a GLOB check, `container 'unknown'` is allowed. `Down` drops `media_streams, media_files,
   items, libraries` (reverse order, read in `migrations/00002_media_catalog.sql:96-100`).
   Evidence: probes above, `TestMediaCatalog*`.
2. `SyncLibraries`: MET. One write transaction on the writer pool; insert with caller IDs,
   update name/kind (and the items' kind on a kind change via `ItemKindForLibrary`), delete
   unconfigured roots by cascade, result ordered by name (case-insensitive, then name, then id).
   Idempotent: second sync generates no IDs and `updated_at` stays (`...TwiceWithTheSameInput...`,
   mutation killed). Rollback of the whole sync on an ID-generator failure and on a failing
   statement in each of the four steps (triggers); invalid input (nil generator, duplicate root,
   invalid definition) wraps `domain.ErrInvalid` before any write.
3. `ListLibraries` / `GetLibrary`: MET. Item counts (3 / 0 and 2), empty catalog gives an empty
   non-nil list, unknown ID wraps `domain.ErrNotFound`, an unknown stored kind wraps
   `domain.ErrInvalid`. The orchestrator decision that `LibrarySummary` moves to the domain in
   M2-18 is not judged here.
4. Integration tests on a real temp SQLite: MET. Migrate up/down/up, sync add/rename/remove,
   cascade of items, files and streams (rows inserted directly), unique root, `foreign_keys`
   enforced on the writer (`TestWriterEnforcesForeignKeysOfTheCatalog`) and on the probe
   connection.
5. BACKLOG "sqlite reader pool" and "sqlite nits": MET.
   - `SetMaxIdleConns(maxOpen)` (`db.go:91`) and `GOMAXPROCS(0)` (`db.go:108`): both pinned by
     `TestReaderPoolKeepsEveryOpenConnectionIdle` (mutations killed).
   - `Ping` reads the file (`ping.go`: `SELECT COUNT(*) FROM sqlite_schema`), pinned by
     `TestPingQueryStartsAReadTransaction` (VDBE `Transaction` opcode present for `pingQuery`,
     absent for the control `SELECT 1`). See observation 2 for the limits of that evidence.
   - Zero `CreatedAt` rejected with `domain.ErrInvalid` and nothing stored (`TestSaveIdentityRejectsZeroValues`).
   - UNC path tests: run and pass on Windows, skip on Linux (`t.Skip`).
   - `t.Run` in the DSN role loop (`TestDataSourceNameSetsRoleSpecificParameters`).
6. sqlc drift check clean: MET (exit 0, no output).
7. Gates (lint, layer rules, race tests, coverage): MET, see Gates table.

## Observations (non-blocking, for the reviewer)

1. **The rollback tests cannot tell "rolled back" from "left open".**
   `TestSyncLibrariesRollsBackEverythingWhenAnIDCannotBeCreated` and `...WhenAStatementFails`
   read the state back through the reader pool, which never sees uncommitted data. Removing
   `rollBack` in `inWriteTransaction` (`library.go:300`) keeps every test green, although the
   single writer connection then stays checked out: a probe running a second `SyncLibraries`
   with a 3 s deadline after a failed one fails with `sync libraries: begin write
   transaction: context deadline exceeded` under the mutation and succeeds with the real
   code. Suggest adding "a sync after a failed sync succeeds" to the existing rollback tests.
2. **`Ping` itself is not pinned to `pingQuery`.** The test checks the constant's program, not
   that `(*DB).Ping` runs it; replacing the body with `db.reader.PingContext(ctx)` passes all
   tests (the constant stays referenced by the test, so `unused` does not flag it). I also
   could not produce a behavioural difference: with a warm idle reader connection in WAL mode
   even a corrupted and truncated main file is not noticed by either variant, and a fresh
   connection fails already while connecting. The implementation is right in principle (it
   starts a read transaction on the file, which is what hits a lock or a vanished share), but
   the evidence for "touches the file" is the opcode check only. A test calling `Ping` while
   another handle holds a lock would be better if SQLite can be made to produce one reliably.
3. **`Down` order is not testable behaviourally.** SQLite drops a parent table with an
   implicit `DELETE` and cascade, so any order works. The reverse order is confirmed by
   reading the file only.
4. `rollBack` is at 75 % coverage: the `sql.ErrTxDone` branch (context cancelled while the
   transaction is open) has no test. Other functions below 90 %: `Open` 88.9 %, `openPool`
   84.6 %, `closePool` 75.0 %, `dataSourceName` 87.5 %, `Migrate` 88.9 %,
   `newMigrationProvider` 71.4 %, `libraries` 85.7 %, `inWriteTransaction` 87.5 %, `update`
   91.7 % (all error branches; package total 94.2 %, threshold 70 %).

## Notes

- Wave 2 files (`internal/adapters/ffmpeg`, `libraryfs`, `restapi`, `instancelock`,
  `domain/playback`, `cmd/mivo`, web `player-core`, `go.mod`, task and BACKLOG docs) changed
  during the run. Not judged here.
- My first whole-repository test run passed `-coverprofile=coverage.out` through PowerShell
  unquoted; it was split at the dot and created a stray file `coverage` in the repository root.
  I deleted it; `git status` shows no `coverage` file. The rerun used a quoted profile path in
  the scratchpad.
- One `golangci-lint run` attempt failed with "parallel golangci-lint is running" (another
  agent's run); the retry finished clean.
- No production code or test in the repository was edited, nothing was added to git.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M2-10
files: docs/reports/M2-10-test.md
tests: go test -race -count=1 and -count=5 ./internal/adapters/sqlite/... PASS (94.2 % coverage); whole repo go test -race ./... PASS (total 96.9 %, gate satisfied); golangci-lint run and fmt --diff clean (package and unscoped); archtest PASS; sqlc diff via Docker clean; Linux run in golang:1.27.3 PASS (UNC tests skip, pass on Windows); schema probes, migration up from an M1 database copy, second Migrate no-op all as expected
findings:
- All 7 acceptance criteria met; no gate failed.
- Test gap (non-blocking): the rollback tests would stay green without any rollback (the writer connection would leak); suggest a "sync after a failed sync" assertion.
- Test gap (non-blocking): nothing pins that (*DB).Ping runs pingQuery; also found no behavioural way to show Ping detects more than the driver ping on a warm WAL connection.
- Down order and the sql.ErrTxDone branch of rollBack are not covered by tests (Down order cannot be, in SQLite).
next: Send to mivo-reviewer; pass observations 1 and 2 on so mivo-backend can add the two assertions (optional, before or together with M2-15).
