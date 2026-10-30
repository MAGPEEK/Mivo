# M2-15 test report: SQLite media catalog repository

Task file `docs/tasks/M2-15-sqlite-media-catalog.md` including the appended "Orchestrator note (from M2-10 review)".
Package under test: `internal/adapters/sqlite` (`queries/media.sql`, `sqlcgen/media.sql.go`, `catalog.go`,
`catalog_writer.go`, `catalog_reader.go`, `catalog_cursor.go`, `transaction.go` and the tests incl.
`catalog_plan_internal_test.go`, `catalog_race_on_test.go`, `catalog_race_off_test.go`, `ping_internal_test.go`,
`transaction_recovery_test.go`). Shares the package with M2-18 (see `docs/reports/M2-18-test.md`).
Round 1. Result: **PASS**.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go build ./...` | PASS | exit 0 |
| `go vet ./...` | PASS | exit 0 |
| `go test -race -count=1 -coverprofile` for `./internal/adapters/sqlite/... ./internal/app/library/... ./internal/domain/library/... ./internal/app/scan/... ./internal/app/playback/...` | PASS | `sqlite 29.462s coverage: 94.8%`, `app/library 100.0%`, `domain/library 100.0%`, `app/scan 97.9%`, `app/playback 98.6%`. The compile-time port assertions hold: `catalog_writer.go:26` `scan.CatalogWriter`, `catalog_reader.go:20-21` `applibrary.ItemReader` and `appplayback.FileSource` |
| Full `go test -race -count=1 -coverprofile ./...` | PASS | every package `ok` |
| Coverage gate `go-test-coverage --config=.testcoverage.yml` (profile from the full run, written to the scratchpad) | PASS | `Package coverage threshold (0%) satisfied`, `Total coverage threshold (70%) satisfied: 96.9% (4258/4394)`; every `internal/domain/...` package 100 % |
| Per-function coverage of the new files | PASS | only error branches below 100 %: `read` 88.9, `readItemDetail` 86.7, `ensureItem` 88.9, `checkNewItem` 89.5, `rollBack` 75.0 (non-`ErrTxDone` rollback failure), `endReadTransaction` 75.0 |
| `golangci-lint run --allow-parallel-runners` | PASS | `0 issues.` |
| `golangci-lint fmt --diff` | PASS | exit 0, no diff |
| sqlc drift: `docker run ... sqlc/sqlc:1.31.1 diff -f internal/adapters/sqlite/sqlc.yaml` | PASS | exit 0, empty output (`sqlcgen/media.sql.go` matches `queries/media.sql`) |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | PASS | both exit 0, no diff |
| archtest `go test -count=1 ./internal/archtest/...` | PASS | `ok` |
| Layer rule (independent) | PASS | `go list -deps ./internal/domain/...` lists no `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app`; `adapters/sqlite` imports `app/scan`, `app/library`, `app/playback` (adapters to app, allowed) |
| Linux run: `docker run golang:1.27.3` with the repo mounted `:ro`, `go test -race -count=1 ./internal/adapters/sqlite/...` | PASS | `ok .../adapters/sqlite 17.123s` |
| Web / E2E / media corpus / Docker image build | n/a | the task touches none of them |
| Stray files / TODO | PASS | no `TODO`/`FIXME` in the touched packages; no coverage or database files in the repo (profiles and probes in the scratchpad) |

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | `FileStates` for change detection | met | `catalog_writer.go:45` returns `[]scan.CatalogFile{ID, ItemID, State}` ordered by relative path; `TestFileStatesReturnsTheFilesOfOneLibraryOrderedByPath`, `...OfALibraryWithoutFilesIsAnEmptyList` |
| 2 | `SaveProbedFile`: one write transaction, new item or kept item, upsert, stream replacement, probe failure stored with `probe_error` and no streams, IDs from the caller | met | `catalog_writer.go:81`; `TestSaveProbedFileStoresANewFile...`, `...OfAStoredFileKeepsItsItemAndReplacesItsStreams`, `...StoresAProbeFailureWithoutStreams`, `...AddsANewFileToAStoredItemOfTheSameLibrary` |
| 3 | `RemoveFiles` deletes files and items left without files in one transaction | met | `catalog_writer.go:100`; `TestRemoveFilesDeletesFilesAndItemsLeftWithoutFiles`, `...CrossesTheBatchBoundaryOf500IDs`, `...RollsBackAFailedStatementAndTheNextWriteSucceeds` |
| 4 | `ListItems` ordered by `(sort_title, id)`, opaque base64url cursor, tampered cursor is `ErrInvalid`, primary file with streams in one extra query | met | `catalog_reader.go:47`, `catalog_cursor.go`; `TestListItemsPagesThroughALibraryInSortOrder`, `TestListItemsRejectsInvalidRequests`, `TestListItemPageSearchesTheSortTitleIndexWithoutSortingItems` (plan: index range, no sort); probes below |
| 5 | `GetItem` with all files and streams; unknown is `ErrNotFound` | met | `catalog_reader.go:76`; `TestGetItemOfAnUnknownIDReturnsNotFound`, `TestListItemsReturnsThePrimaryFileOfAnItemWithSeveralFiles` |
| 6 | `PlayableFile` with streams and library root; unknown is `ErrNotFound` | met | `catalog_reader.go:96`; `TestPlayableFileReturnsTheFileWithStreamsAndTheLibraryRoot`, `...ReturnsAFileWhoseProbeFailed`, `...OfAnUnknownIDReturnsNotFound` |
| 7 | Integration tests on real temp SQLite incl. 1 000 items under 200 ms (log the time) | met | `TestListItemsOfAThousandItemsIsFast`: `listed 1000 items with 3 streams each in 2 pages in 26.5784ms (race detector false)`; under `-race` it only logs (`549.7656ms`), enforced in the non-race build via `catalog_race_off_test.go` |
| 8 | sqlc drift check clean | met | see gates (exit 0, empty diff); command is in `docs/development.md:147` |
| 9 | Gates: lint, layer rules, `go test -race ./internal/adapters/sqlite/...`, coverage gate | met | see gates |
| N1 | Same-library invariant enforced in the repository (same transaction) and tested | met | `checkProbedFile` (`catalog_writer.go:117`), `apply`, `ensureItem`; `TestSaveProbedFileRejectsInvalidRecordsAndChangesNothing`; probe below |
| N2 | "Write after a failed write succeeds" for SyncLibraries and the catalog writes; `sql.ErrTxDone` branch of `rollBack` covered; `(*DB).Ping` runs `pingQuery` pinned | met | `TestSyncLibrariesAfterAFailedSyncSucceeds`, `TestSaveProbedFileRollsBack...AndTheNextWriteSucceeds`, `TestRemoveFilesRollsBack...`, `TestEndingAnEndedTransactionIsNoError`, `TestPingRunsThePingQueryOnTheReaderPool` (recording driver sees exactly `pingQuery`). `rollBack` is 75 % (only the non-`ErrTxDone` failure is uncovered) |
| N3 | One method per use; writer and reader split; writes via writer pool, reads via reader pool; `relative_path` stored as returned by `library.ParseRelativePath` | met | `CatalogWriter` (writer pool + reader for `FileStates`) and `CatalogReader` (reader pool, read transaction per call); `checkProbedFile` calls `ParseRelativePath`, `fileFromRow` re-checks it on read |

## Independent probes (scratch module in the scratchpad, temp DBs, repo untouched)

All run with `-race`; file `scratchpad/qa-m215/probe/probe_test.go`.

| Probe | Result | Evidence |
|---|---|---|
| Same-library rule violations leave the DB unchanged | PASS | 8 cases (new file in B naming a stored item of A, `File.LibraryID != Item.LibraryID`, stored file id re-saved as another library, re-saved under another item, new file in A on a stored item of B, `File.ItemID != Item.ID`, wrong item kind, unknown library): all rejected (`ErrInvalid`, unknown library `ErrNotFound`); full table dump of items, media_files and media_streams identical before and after each; a following write succeeded within 5 s (no leaked writer connection) |
| New file id at a stored path | PASS | `ErrConflict` (`write file "a/one.mkv": another file is stored at this path: conflict`) for both a new item id and an existing item id; the half-inserted new item is rolled back (dump identical, item count 2 after the next successful write) |
| Keyset paging over 1 000 items with 10 duplicated sort titles (case variants incl. `Über`/`über`, `Cæsar`) | PASS | limits 1, 7, 37, 100, 333, 500: 1000/143/28/10/4/2 pages; every item exactly once, strictly increasing `(sort_title, id)`, each with primary file and its stream, non-last pages full |
| Cursor round trip incl. > 512 chars | PASS | 300 CJK runes (900 bytes) title: cursor length 1250, paging with limit 1 returned all 4 items in order |
| Tampered cursors | PASS | `!!!not*base64!!!`, padding `=`, std alphabet `+/`, truncated, no separator, empty id, trailing space: all `ErrInvalid`. Unknown library with a valid request: `ErrNotFound` |
| Undecodable stored row through `app/library` | PASS | unknown item kind, unknown container, malformed frame rate, `../evil.mkv` path: `ListItems` and `GetItem` return a plain error ("stored row cannot be decoded ...") with `errors.Is(ErrInvalid) == false` and `ErrNotFound == false`; a bad cursor and `limit=501` still return `ErrInvalid`, an unknown library `ErrNotFound` |
| Mutation check (copy in the scratchpad, repo untouched) | PASS | removing the rollback in `inWriteTransaction` fails 4 tests (`TestSyncLibrariesAfterAFailedSyncSucceeds`, the two `...RollsBack...NextWriteSucceeds`, `...AnotherFileAtAStoredPathIsAConflict`); dropping the stored-item library check, the stored-file owner check or the file/item library check each fails `TestSaveProbedFileRejectsInvalidRecordsAndChangesNothing`; not trimming the look-ahead row fails 2 paging tests; accepting a cursor without separator fails `TestListItemsRejectsInvalidRequests` |

## Findings (none blocking)

- `catalog_cursor.go:36`: `base64.RawURLEncoding.Strict()` still ignores `\r` and `\n` (Go's decoder skips them), so
  `good + "\n"` decodes to the same key and returns the normal page instead of `ErrInvalid`. Harmless (same position,
  no other cursor reachable by this), but the doc comment "A cursor that encodeCursor cannot have produced wraps
  domain.ErrInvalid" is slightly too strong. Optional: reject cursors containing `\r`/`\n`.
- Cursor length: a cursor from a long CJK title is 1250 characters in the probe, but `api/openapi.yaml:179,764` still
  has `maxLength: 512` for `cursor` and `nextCursor`. This is already tracked (decision 2026-10-30 in
  `docs/tasks/M2-26-restapi-library-endpoints.md:43-46`: raise to 2048 and test a cursor > 512 characters). It must not
  be dropped in M2-26; a cursor of an NTFS name of 255 CJK characters is about 1 070 characters, below 2048.
- The 200 ms budget for 1 000 items is enforced only in the non-race build (race run logged 549.8 ms); non-race
  measured 26.6 ms.

## History

- Round 1: PASS (this report).
