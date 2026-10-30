# M2-19 Use case: scan a library in the background
Owner: mivo-backend
Depends on: M2-02, M2-03
Files/areas: `internal/app/scan/**` (new package)

## Goal
A scan walks a library, probes only new or changed video files, stores the results, removes
vanished files and reports progress – in the background, one scan per library, stoppable on
shutdown (ADR 0022).

## Acceptance criteria
- [ ] Ports declared here: library lookup (`GetLibrary`), walker (`Walk(ctx, root, visit)`),
      path resolver (`Resolve(root, relativePath)`), prober (`Probe(ctx, absolutePath)`), catalog
      writer (`FileStates`, `SaveProbedFile`, `RemoveFiles`), ID generator, clock.
- [ ] `Service.Start(ctx, libraryID) (Status, error)`: unknown library → `ErrNotFound`; a scan
      already running for that library returns its status without starting another; otherwise
      starts a goroutine owned by the service and returns `running`.
- [ ] The scan: walk → diff with `library.Changed` → probe new/changed files with at most two
      concurrent probes (named constant) → save; files not seen → remove; files without a video
      stream are skipped and counted; a probe error is saved as `probe_error` and counted as
      failed, never aborting the scan; a walker error `ErrUnavailable` ends the scan as failed with
      code `root_unavailable`.
- [ ] `Status(libraryID)` returns state (`idle|running|failed`), start/finish time and counts
      (added, updated, removed, unchanged, failed, skipped) of the current or last scan.
- [ ] `Close(ctx)` cancels running scans and waits for them (bounded by ctx); `Start` after
      `Close` → `ErrUnavailable`.
- [ ] `StartAll(ctx)` starts a scan for every library (used at startup if OD-3 says so).
- [ ] Tests with fakes: first scan adds, rescan unchanged probes nothing, changed file re-probed,
      removed file deleted, probe error counted, root unavailable, concurrency limit observed
      (fake prober records max parallelism), cancellation mid-scan leaves no goroutine
      (`-race`, `goleak`-style check without a new dependency, e.g. waiting on `Close`).
      Coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + `go test -race ./internal/app/...` + coverage gate.

## Notes
- The scan goroutine must use a service-level context derived at construction (or passed to
  `Start` with `context.WithoutCancel` plus the service's own cancel), never the HTTP request
  context.
- Log one info line per finished scan with the counts, and per-file problems at debug with
  relative paths only.
- Titles come from `library.TitleFromFileName`.

## Orchestrator note (from M2-03)
- `internal/domain/library` defines `ItemKind` with the values `movie` and `episode`, plus
  `ItemKindForLibrary` (`movies` → `movie`, `shows` → `episode`) and `ParseItemKind`. Use exactly
  these values (e.g. the `items.kind` CHECK constraint) and derive the kind via `ItemKindForLibrary`.
- `ParseRelativePath` returns a plain `string` (the input unchanged) and rejects `.` segments and
  drive-relative prefixes such as `C:x`.
