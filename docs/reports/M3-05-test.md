# M3-05 test report

Round: 1 | Result: PASS

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` | exit 0 |
| `go test -race -count=1 -coverprofile=coverage.out ./...` | exit 0, no FAIL |
| `go-test-coverage --config=.testcoverage.yml` | PASS, total 95.8 % (>= 70 %); sqlite package 94.8 %; `internal/domain/...` 100 % |
| `golangci-lint run` | 0 issues |
| `golangci-lint fmt --diff` | clean (exit 0) |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | clean |
| sqlc 1.31.1 `generate` then `diff` (Docker) | exit 0; regeneration is idempotent (the `git diff --stat` of `sqlcgen` is identical before and after, so the committed output matches the generator) |
| Corpus integration test | skipped: FFmpeg is installed but `testdata/media` is absent, and the task does not touch playback |
| Layer rule | `internal/archtest` passes inside the full run; task changes no domain code |

## Acceptance criteria
1. Migration `00003_catalog_v2.sql` drops and recreates the four tables, then creates the eight new ones; Down restores the 00002 shape. Met (file read; `TestMigrationDownToVersion2RestoresTheVersion2Schema` PASS).
2. CHECKs for the closed sets, non-empty checks for growing sets, date GLOB. Met (migration lines 35, 49-51, 58, 66, 102, 146, 210, 248; `TestCatalogV2ConstraintsRejectInvalidRows` PASS).
3. `items` indexes: unique `items_match_key`, partial `items_top_level`, `items_children`. Met (lines 78-83).
4. `artwork`: location-group CHECKs, partial unique `artwork_selected`, CASCADE from items, media files and libraries. Met (lines 206-242; `TestCatalogV2RowsCascadeFromFilesItemsAndLibraries` PASS).
5. Every FK column leads an index; all tables STRICT; BACKLOG nit row removed; `sort_title`, `frame_rate` and HDR comment done. Met. The leftover nits (`COLLATE NOCASE`, roots query) are noted in M3-05c.
6. The M2 writer works with the smallest change; the existing tests are green. Met (`TestSaveProbedFileKeysANewItemByItsFilePathAndKeepsTheV2Defaults` PASS, full `go test -race ./...` green).
7. Schema tests (empty DB, 00002 DB with rows, down, rejects, partial index). Met (`TestMigrateCreatesTheSchemaAndASecondRunIsANoOp`, `TestMigrateDropsTheCatalogOfAVersion2Database`, `TestMigrationsCanBeRolledBackAndReapplied`, `TestCatalogV2ConstraintsAcceptDomainValues`; all PASS).
8. sqlc output committed and `sqlc diff` clean. Met (exit 0).
9. Gates (vet, lint, fmt, race tests, coverage). Met.

## Findings
none blocking. Note: `coverage.out` is a generated file in the worktree; confirm it is gitignored before the commit.

## History
- Round 1: PASS
