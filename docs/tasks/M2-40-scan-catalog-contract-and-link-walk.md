# M2-40 Scan: catalog contract, `scan_canceled`, bounded link walk
Owner: mivo-backend
Depends on: M2-12, M2-15, M2-19 (committed)
Files/areas:
- `internal/app/scan/ports.go`, `failure.go`, `service.go` (only `runScan`/`describeEnd`
  call), `fakes_test.go`, `run_test.go`, `service_test.go`, `harness_test.go`
- `internal/adapters/sqlite/catalog_writer.go` (doc comment only) and
  `catalog_writer_test.go` (only if a contract case is missing)
- `internal/adapters/fs/libraryfs/**` (`walk.go`, `doc.go`, `platform_windows.go`,
  `platform_unix.go`, their tests)
- `docs/tasks/BACKLOG.md` (only the rows below)

## Goal
The scan's catalog port states every error the sqlite adapter returns, and the scan's fake
enforces the same rules, so use-case tests cannot pass against behaviour the real catalog
refuses. A shutdown during a scan is always reported as `scan_canceled`, never as a catalog
failure. A library with several directory links back to an ancestor on a file system without
directory ids can no longer make a scan run (practically) forever.

Closes BACKLOG rows (delete or shorten them in `docs/tasks/BACKLOG.md` when done, touch no
other row):
- `app/scan` | "ports.go/fake: document and enforce the remaining sqlite adapter rules …;
  report `scan_canceled` whenever the scan context is done …; prune trackers/withoutVideo for
  removed libraries (M7 …)" (M2-19 review r2): **keep the row with only the pruning part**,
  due M7.
- `libraryfs` | "unknown directory ids + several directory links back to an ancestor grow the
  pending-link queue exponentially …" (M2-12 review r3): delete.

The decisions are recorded in ADR 0022 "Notes (M2-39, M2-40)"; do not edit the ADR. Report any
deviation.

## Acceptance criteria
- [ ] `scan.CatalogWriter.SaveProbedFile` (ports.go) documents, besides the `ProbedFile` record
      rules: `domain.ErrNotFound` for an unknown library; `domain.ErrConflict` when another file
      ID is stored at the same relative path of the library; `domain.ErrInvalid` when a stored
      file ID belongs to another item or library, or a stored item to another library. The
      sqlite adapter's doc comment (`catalog_writer.go`) says the same in the same order.
- [ ] The in-memory `memoryCatalog` (fakes_test.go) enforces each of these rules with the same
      error and records a violation, which the harness keeps asserting never happens in a scan.
      Table test of the fake: one row per rule (unknown library, other file ID at a stored
      path, stored file moved to another item, stored file moved to another library, new file
      of a stored item in another library) plus the existing record rules.
- [ ] Every rule has a sqlite adapter test: name the existing ones in the REPORT
      (`TestSaveProbedFileRejectsInvalidRecordsAndChangesNothing`,
      `TestSaveProbedFileOfAnotherFileAtAStoredPathIsAConflict`) and add a case only for a rule
      without one.
- [ ] A scan whose context is done ends with `scan_canceled` whatever error the interrupted
      step returned. Test: a catalog fake whose `SaveProbedFile` (and one for `FileStates`)
      cancels the scan context (e.g. via `Close`) and returns a plain error such as
      `errors.New("interrupted (9)")` without `context.Canceled` → final status `scan_canceled`,
      finish line at info level without a `cause`. A failure while the context is still alive
      keeps its code (`catalog_failed`).
- [ ] libraryfs: a directory reached through a link whose identity is unknown is not listed,
      is logged at info level with its root-relative path and is reported as skipped;
      directories of the real tree with an unknown identity are still listed. Test with the
      injectable `identify` returning unknown ids for every directory and a tree in which one
      directory holds three relative links back to the root: the walk finishes, visits each
      real-tree file exactly once and reports the three link paths as skipped (add a timeout
      guard so a regression fails instead of hanging).
- [ ] Test for a link-reached duplicate with a known identity: listed once, logged at debug,
      not reported as skipped.
- [ ] The comments that promise "cross-linked folders cannot multiply the walk" (`doc.go`,
      `Walk` doc in `walk.go`) and the `maxDirectoryDepth` comment match the new rule.
- [ ] Nits from the row: on Windows the 64-bit file index is used as a fallback only when
      `FileIdInfo` is not supported (`ERROR_INVALID_PARAMETER`/`ERROR_NOT_SUPPORTED` or what
      the code already classifies as such); other errors are returned (test through an
      injectable query where feasible, else document why not); the `videoFileInfo` comment in
      `platform_unix.go` reflects that `DirEntry.Info` is confined in Go 1.27; the misleading
      comment in `walk_internal_test.go` is fixed.
- [ ] Tests: `go test ./internal/app/scan/... ./internal/adapters/sqlite/...
      ./internal/adapters/fs/libraryfs/...` green on Windows; the libraryfs unix tests green in
      Docker (`go test -race ./internal/adapters/fs/libraryfs/...` via the Docker line).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`, coverage gate
      (app ≥ its threshold, libraryfs and sqlite unchanged or higher) pass.

## Notes
- `describeEnd` can take the scan context (or a `canceled bool` computed by `runScan` from
  `service.scanContext.Err()`); cancellation still wins over any `failure` code.
- Reporting the skipped link path keeps catalog entries that an earlier scan stored below it
  (ADR 0022 "Skipped paths"); that is the safe default and matches the collision rule.
- Checking `throughLink` belongs in `admit` next to the existing identity and depth checks;
  keep `admit` within the complexity limit (a helper per rule is fine).
- No change to the sqlite writer's behaviour is expected; if a rule turns out not to be
  enforced there, report it and fix it in this task with a failing test first.
- Pruning trackers and the no-video memory for removed libraries stays M7 (runtime library
  edits).
