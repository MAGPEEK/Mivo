# M1-08 test report

Status: PASS (round 1)

Scope: `internal/adapters/sqlite/**` (incl. `migrations/`, `queries/`, `sqlc.yaml`, `sqlcgen/`), `go.mod`, `go.sum`.
Gates were run on `./internal/adapters/sqlite/...` plus `./internal/archtest/...` only, because
`scripts/gen-testmedia` is mid-change (M1-19) and M1-09/M1-15 are in progress. Web, media and Docker
gates are not applicable.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/adapters/sqlite/... ./internal/archtest/...` | exit 0 |
| tests | `go test -race -count=3 -cover ./internal/adapters/sqlite/... ./internal/archtest/...` | `ok sqlite 3.057s coverage: 91.1%`, `ok archtest coverage: 95.4%`; also stable on a further single run |
| lint | `golangci-lint run ./internal/adapters/sqlite/... ./internal/archtest/...` | `0 issues.` (golangci-lint 2.15.0, incl. depguard, sqlclosecheck, rowserrcheck) |
| format | `golangci-lint fmt --diff ./internal/adapters/sqlite/...` | exit 0, no diff |
| coverage gate | `go test -race -coverprofile=<scratch>/cov-sqlite.out ./internal/adapters/sqlite/...`; `go tool '-modfile=tools/go.mod' go-test-coverage '--config=.testcoverage.yml' '--profile=<scratch>/cov-sqlite.out'` | `Total coverage threshold (70%) satisfied: PASS`, total 91.1% (82/90), `sqlcgen` excluded as configured |
| tidy | `go mod tidy -diff` / `go -C tools mod tidy -diff` | both exit 0, no diff; `tools/go.mod` unchanged |
| vuln | `go tool '-modfile=tools/go.mod' govulncheck ./internal/adapters/sqlite/...` | `No vulnerabilities found.` |
| layer rule | `go list -deps ./internal/domain/...` filtered for `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` | no matches; archtest ok (domain untouched by this task) |
| sqlc | `sqlc` not on PATH; not downloaded (per instruction). Verified by reading | see criterion 4 |

Per-function coverage (package without sqlcgen): `Close`, `Ping`, `NewIdentityRepository`, `LoadIdentity`,
`SaveIdentity`, `connectionParameters` 100 %; `Open` 88.9 %, `Migrate` 88.9 %, `dataSourceName` 87.5 %,
`openPool` 81.8 %, `closePool` 75 %, `newMigrationProvider` 71.4 % (the uncovered branches are defensive
error returns that cannot be triggered with a real database: `filepath.Abs`, `fs.Sub`, `NewProvider`,
second-pool open failure). Every exported function is covered, including error paths.
`sqlcgen` shows 0 % in `go test` (cross-package; exercised through the repository, excluded in `.testcoverage.yml`).

## Acceptance criteria

1. Two pools, pragmas: MET. `db.go:58-68` opens writer first, then reader; `db.go:97-105` writer
   `SetMaxOpenConns(1)` (`writerMaxOpenConnections`), reader `runtime.NumCPU()`. `dsn.go:68-83` sets
   `_journal_mode=WAL`, `_busy_timeout=5000`, `_foreign_keys=1`, `_synchronous=NORMAL` on both, `_txlock=immediate`
   on the writer only and `_query_only=1` on the reader only. `TestOpenAppliesPragmasOnEveryConnectionOfBothPools`
   reads all five PRAGMAs on the writer (1 conn) and on two distinct reader connections (want wal/5000/1/1 and
   query_only 0 / 1); `TestOpenLimitsWriterToOneConnection`; `TestWriterTransactionsTakeTheWriteLockAtBegin`
   (a second connection with busy_timeout 0 gets "locked" while a writer tx is open, proving `_txlock=immediate`);
   `TestReaderPoolRejectsWrites`. All pass under `-race -count=3`.
2. `file:` URI with special characters: MET. `dsn.go:51-66` builds `file:///C:/...` from `filepath.Abs` + `ToSlash`,
   percent-encoded through `url.URL`. `TestDataSourceNameIsAnAbsoluteFileURIThatRoundTripsThePath` (plain, spaces and
   `#`/`%20`/`ä`, relative) and `TestOpenUsesTheExactPathWithSpacesAndURICharacters` (real Open+Migrate+Close in a
   directory `Jane Doe #1 %20 ä`; only `server.db` exists afterwards, i.e. no decoded or truncated name). Run on
   Windows 11 with drive-letter temp paths. (`?` in a path is not tested; `url.URL` encodes it, see Remarks.)
3. Migrations: MET. `migrate.go:27-45` uses `goose.NewProvider(goose.DialectSQLite3, db.writer, fs.Sub(embed), WithDisableGlobalRegistry(true), WithSlog(db.logger))`;
   grep over the package for `SetBaseFS|SetLogger|SetDialect|goose.Up|fmt.Print|log.Print|os.Stdout|os.Stderr|init()`
   returns nothing, so no global goose state and no stdout/stderr output. `TestMigrateCreatesTheSchemaAndASecondRunIsANoOp`:
   two runs, `MAX(version_id)` = 1, exactly 1 applied row, table `server_identity` exists, exactly one log record
   (the applied migration, via the injected logger; the second run logs nothing). `TestMigrateFailsWithACancelledContext`.
   `TestMigrationsCanBeRolledBack` runs the `Down` section.
4. Migration file and sqlc: MET (sqlc regeneration verified by reading, not executed). `00001_server_identity.sql`:
   `id INTEGER PRIMARY KEY CHECK (id = 1)`, `server_id TEXT NOT NULL`, `created_at INTEGER NOT NULL`, `) STRICT;`,
   `-- +goose Down` with `DROP TABLE server_identity;`. `TestServerIdentityTableHoldsASingleRow` shows the CHECK rejects id 2.
   `sqlc.yaml`: engine `sqlite`, schema `migrations`, queries `queries`, out `sqlcgen`, version 1.31.1 pinned in the header.
   All three `sqlcgen/*.go` files carry `// Code generated by sqlc. DO NOT EDIT.` and `sqlc v1.31.1`.
   Consistency with `queries/server_identity.sql`: `GetServerIdentity :one` -> `GetServerIdentityRow{ServerID string; CreatedAt int64}`
   and SQL text identical; `InsertServerIdentity :execrows` -> `(int64, error)` via `RowsAffected()`, params `ServerID`, `CreatedAt`,
   SQL text identical (no trailing `;`, `-- name:` line kept, the leading query comment becomes the doc comment);
   `models.go` `ServerIdentity{ID, ServerID int64/string, CreatedAt int64}` matches the table. All files ASCII, LF only
   (`.gitattributes` eol=lf). Residual risk: byte-identity of a real `sqlc generate` run could not be confirmed here;
   the orchestrator should run `sqlc diff -f internal/adapters/sqlite/sqlc.yaml` once with the release binary when available.
5. `IdentityRepository`: MET. `identity.go`. Empty table -> `domain.ErrNotFound` (`TestLoadIdentityFromAnEmptyTableReturnsNotFound`);
   round trip of ID and `CreatedAt` truncated to ms and UTC from a non-UTC input with sub-ms digits
   (`TestSaveThenLoadRoundTripsIDAndCreatedAtInMillisecondsUTC`); second save -> `domain.ErrConflict` and the first identity
   is still loaded unchanged (`TestSavingASecondIdentityReturnsConflictAndKeepsTheFirst`); never overwrites by
   construction: `INSERT ... ON CONFLICT (id) DO NOTHING` with `rowsAffected == 0` -> ErrConflict (`identity.go:58-68`);
   zero ID -> `ErrInvalid`; malformed stored ID -> `ErrInvalid`; cancelled context and closed DB for both operations.
   Concurrency: `TestConcurrentSavesStoreExactlyOneIdentity` (3 goroutines, exactly 1 success, others ErrConflict) and
   `TestEnsureIdentityAcrossTwoOpenDatabasesAgreesOnOneID` (two independent `DB` instances on one file run
   `EnsureIdentity` concurrently, both adopt the same ID, a restart keeps it). Green in all 3 `-race` iterations.
6. `Ping` / `Close`: MET. `ping.go:19-33` pings `db.reader` only; on failure logs the raw error via the injected logger
   (`database ping failed`, attr `error`) at Debug for `context.Canceled`, Warn otherwise (deadline exceeded, closed DB),
   and returns a wrapped error. `TestPingFailureIsReturnedAndLoggedAtItsLevel` asserts exactly one record with the expected
   level and raw error text for cancelled / deadline / closed; `TestPingSucceedsOnAnOpenDatabase` asserts no logging on
   success. `Close` (`db.go:72`) returns `errors.Join` of reader and writer close errors; `TestCloseClosesBothPoolsAndIsIdempotent`.
7. Compile-time assertions: MET. `db_test.go:18-21` `var _ appsystem.IdentityStore = (*sqlite.IdentityRepository)(nil)` and
   `var _ appsystem.DatabaseChecker = (*sqlite.DB)(nil)`.
8. Tests: MET. Integration tests against real SQLite in `t.TempDir()`, error paths (closed DB, cancelled/expired context,
   empty path, missing directory, directory as path) covered; no mocks.
9. Gates: MET for the scoped set, see table (lint, tests, coverage with `sqlcgen` excluded).

Orchestrator notes: ErrConflict never overwrites (tested incl. concurrent two-instance case); raw ping error logged by
the adapter; cancellations at debug, other failures at warn. All MET.

## Remarks (non-blocking)

- No test pins that `Ping` uses the reader pool (e.g. by holding the single writer connection and expecting Ping to succeed).
  The code does (`ping.go:20`); a regression to the writer pool would not be caught. Suggest adding that test.
- No test asserts the `STRICT` keyword (`sqlite_schema.sql` / `PRAGMA table_list` strict flag); it is present in the migration (`00001_server_identity.sql:10`).
- `?` in the database path is not covered by a test, only `#`, `%20`, spaces, `ä`. `url.URL` percent-encodes `?`, so the
  risk is low.
- Repo-wide gates (`go build ./...`, `golangci-lint run`, total coverage) were not run because of parallel work in other
  tasks.

## History

- Round 1: PASS (all scoped gates green, all criteria met; sqlc regeneration verified by reading only).

REPORT
status: PASS
task: M1-08
files: docs/reports/M1-08-test.md
tests: go vet (exit 0); go test -race -count=3 -cover sqlite+archtest ok (sqlite 91.1 %, archtest 95.4 %); golangci-lint 0 issues; fmt --diff clean; go-test-coverage on sqlite-only profile PASS (total 91.1 %, sqlcgen excluded); go mod tidy -diff clean (root and tools); govulncheck no vulnerabilities
findings:
- all acceptance criteria and orchestrator notes met; sqlc output checked by reading against queries/migrations (sqlc binary unavailable, v1.31.1 header on all generated files)
- non-blocking: no test that Ping uses the reader pool, none for STRICT, none for '?' in path
- non-blocking: run `sqlc diff -f internal/adapters/sqlite/sqlc.yaml` once with the release binary when convenient
next: proceed to mivo-reviewer for M1-08
