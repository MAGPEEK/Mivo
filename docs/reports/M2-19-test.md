# M2-19 test report: scan library in the background (use case)

Package under test: `internal/app/scan` (doc.go, ports.go, status.go, failure.go, service.go, run.go, plan.go,
probe.go and tests) plus `docs/adr/0022-media-library-model-and-scanning.md` ("Notes (M2-19)"). Task file
`docs/tasks/M2-19-app-scan.md` including all appended notes.
Round 2. Result: **PASS**.

## Gates (round 2)

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./internal/app/scan/...` | PASS | exit 0, no output |
| `go test -race -count=5 -coverprofile ./internal/app/scan/...` | PASS | `ok ... scan 2.496s coverage: 97.9% of statements` |
| Coverage gate (>= 90 %) | PASS | 97.9 %; lowest: `failure.go:Error` 0.0 % (`(*failure).Error`, only reached when a failure is printed; the type is always unwrapped through `errors.AsType`), `recordProbeFailure` 83.3 %, `describeEnd` 85.7 %, `record` 85.7 %, `identify` 85.7 %, `StartAll` 88.9 % |
| `go test -race -count=20` and `-count=20 -cpu 1,4` (whole package) | PASS | `ok ... scan 2.992s`, `ok ... scan 4.057s` |
| `go test -race -count=20 -run 'TestClose\|TestStart\|TestNewService\|TestStatus\|TestTheProbe'` (service tests incl. the 4 new ones) | PASS | `ok ... scan 3.010s` |
| `golangci-lint run --allow-parallel-runners ./internal/app/scan/...` | PASS | `0 issues.` |
| `golangci-lint fmt --diff ./internal/app/scan/...` | PASS | exit 0, no diff |
| archtest `go test -count=1 ./internal/archtest/...` | PASS | `ok ... archtest 1.475s` |
| Layer rule `go list -deps ./internal/app/scan` | PASS | project deps: `internal/domain`, `internal/domain/library`, `internal/domain/media`; `go list -deps -test` lists no `adapters` package |
| `go build ./internal/app/...` | PASS | exit 0 |
| `go build ./internal/adapters/sqlite/` (M2-15, round 2 in progress) | PASS | exit 0; `internal/adapters/sqlite/catalog_writer.go:25` `var _ scan.CatalogWriter = (*CatalogWriter)(nil)` compiles against the new `app/scan` (signatures unchanged) |
| Web / E2E / media corpus / Docker | n/a | the task does not touch them |
| Stray files | PASS | `ls internal/app/scan` shows the 13 expected files; no TODO/FIXME; no coverage output in the repo (profile written to the scratchpad); scratch tests only in the scratchpad |

## Acceptance criteria and notes

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Ports: library lookup, walker, path resolver, prober, catalog writer (`FileStates`, `SaveProbedFile`, `RemoveFiles`), ID generator, clock | met | `ports.go`; clock is `Dependencies.Clock func() time.Time`. `ports.go` now documents the `ProbedFile` rules and that `RemoveFiles` is idempotent (unknown and foreign IDs ignored); the fake enforces both and records violations that the harness asserts empty at cleanup |
| 2 | `Start`: unknown library -> `ErrNotFound`; running scan returns its status without a second one; else goroutine, returns `running` | met | `TestStartOfAnUnknownLibraryIsNotFound`, `TestStartWhileAScanRunsReturnsItsStatusWithoutAnotherScan`, `TestStartWhileAScanLogsItsEndDoesNotStartAnother`; probe 3 below |
| 3 | Pipeline: walk -> `library.Changed` diff -> probe with at most two concurrent probes -> save; unseen -> removed; no video -> skipped + counted; probe error saved as `probe_error`, counted failed, no abort; walker `ErrUnavailable` -> `root_unavailable` | met | `plan.go`, `probe.go:MaxConcurrentProbes = 2` (now a service-wide semaphore `probeSlots`), tests listed in round 1 plus `TestTheProbeLimitIsSharedByAllScans`; probes 1, 2, 4 below |
| 4 | `Status`: state, start/finish time, counts | met | `status.go`; `final`/`publish` split: `running` is decided by `done`, final status published after logging; probe 3 (32 concurrent `Start`s and `Status` during the finish line all see `running`, `FinishedAt` zero) and the consistency hammer |
| 5 | `Close(ctx)` cancels and waits (bounded by ctx); `Start` after `Close` -> `ErrUnavailable` | met | `TestCloseWaitsForAScanThatLogsItsEnd` (Close waits for a scan still in its finish line and is bounded by ctx), `TestCloseCancelsARunningScanWithoutRecordingFailures`, `TestStartAfterCloseIsUnavailable` |
| 6 | `StartAll` | met | `TestStartAllScansEveryLibrary`, `TestStartAllKeepsAScanThatAlreadyRuns`; probe 6 |
| 7 | Tests with fakes incl. concurrency limit and no goroutine left; coverage >= 90 % | met | 97.9 %; `TestProbesRunWithBoundedConcurrency`, `TestCloseLeavesNoGoroutineBehind` |
| 8 | Gates | met | gate table |
| N1 | Service-level context, never the request context | met | `TestStartIgnoresTheCallersCancellation` |
| N2 | One info line per finished scan with counts; per-file problems at debug, relative paths only | met | `service.go:logFinished` (info; warn plus `code` and `cause` on failure; `logContext` is never cancelled so the line of a `Close`d scan is still written - probe 2 confirms `scan_canceled` is logged), `run.go:logFile`; see observation 1 for the debug `error` attribute |
| N3 | Titles via `library.TitleFromFileName`, kind via `ItemKindForLibrary` | met | `run.go:newRecord`; `TestShowsLibraryCreatesEpisodes` |
| N4 | M2-11 prober error contract | met | `probe.go:probeFailure`; round-1 probes re-run, all pass |
| N5 | No import of `adapters/ffmpeg` | met | `go list -deps` |
| N6 | M2-12 notes: keep entries below skipped prefixes, lost root deletes nothing, walker port shape | met | `plan.go:belowAny` now also normalises trailing `/`; probes 1 and 5 |
| ADR | ADR 0022 "Notes (M2-19)" | met | lines 195-217 cover the shared probe limit, delete-only-after-complete-walk, unresolvable/failed files stored as `probe_failed`/`probe_timed_out`, in-memory files without video (probed once more after a restart), the error codes with the cause-without-absolute-paths rule, and `finishedAt` semantics; consistent with the code |

## Scratch probes (own tests, `go test -race -overlay`, nothing written into the repository)

Files only in the scratchpad: `...\scratchpad\m219\zz_qa_test.go`, `zz_qa2_test.go`, `overlay.json`. They reuse the
package's fakes. After the first run the probes below were repeated with `-count=10 -race` (all pass).

Round-1 probes re-run (all PASS, only difference: trailing-slash case is now protected, see 5):
- skipped `Locked` keeps `Locked`, `Locked/x.mkv`, `Locked/y/z.mkv` and removes the sibling `Lockedness*`; `a/b`; `.` and `""` keep everything; `nil` removes everything unseen.
- root lost before the walk, mid-walk (with and without files and skipped paths), at resolve, generic walker error: no removals, no saves where the walk did not complete, codes `root_unavailable` / `scan_failed`.
- ffprobe missing (fresh scan and re-probe of 10 stored rows): `prober_unavailable`, `Failed` 0, no saves, stored rows keep `ProbeErrorNone`.
- prober `Canceled` with a live context: `scan_canceled`, `Failed` 0, no `probe_error` rows; `DeadlineExceeded`: `probe_timed_out`, `Failed` 1, other file added, scan `idle`; `Close` mid-scan: `scan_canceled`, zero saves.
- 64 concurrent `Start`s: one walk, identical `StartedAt`; `Close` x3 returns nil; 30 rounds of 8x(`Start`+`StartAll`) racing 8x`Close`: no panic/race, goroutines back to baseline.
- 40 files, one library: peak exactly 2 probes.

New round-2 probes (all PASS):
1. Two libraries at once -> max 2 probes in total: three libraries (15 files each) via `StartAll`, with an independent
   atomic counting prober (not the package fake): `max concurrent probes = 2`, 45 probes, every library `Added: 15`.
   `TestTheProbeLimitIsSharedByAllScans` in the package agrees.
2. Error text containing an absolute root never appears in a log record. Roots `/srv/secret-nas/lib` and
   `D:\Secret NAS\lib`; error text carrying the root injected at: walker (missing root, generic error, wrapped
   `ErrUnavailable`), resolver (`ErrUnavailable`, `ErrNotFound`, `ErrForbidden`, plain error), prober
   (`ErrUnavailable`, `Canceled`), and a stuck probe ended by `Close`. No log record contains the root or `/abs/`; the
   finish line carries `code=` and the fixed cause (`library folder is missing or unreadable`, `walking the library
   folder failed`, `path could not be resolved`, ...). A `Close`d scan still logs `scan_canceled`. Exceptions: see
   observations 1 and 2 (not covered by the stated claim, not reachable with the real adapters).
3. A `Start` racing the finish line returns `running`, no second walk: the log handler blocks the finish record; 32
   concurrent `Start`s during it all return `running`, `Status` is `running`, the walker was called once; after release
   the status is final (`FinishedAt` set) and a later `Start` walks again (2 walks). Run for a successful scan and for
   a failed one (`root_unavailable`). A hammer of 8 goroutines calling `Start`+`Status` for 400 ms under `-race` saw no
   observation of `running` with a finish time or of a finished state without one.
4. Unchanged file without video is not probed again: first scan `Added 1, Skipped 1` (2 probes); rescans `Unchanged 1,
   Skipped 1` with no new probe (twice); size change -> probed again and stored; stored file that lost its video stream
   -> removed from the catalog, counted skipped, remembered (no further probe); file vanishes -> forgotten, reappears
   identical -> probed once; the memory is per library (same path in another library is probed); a failed scan (root
   lost) does not wipe the memory.
5. Trailing-slash skipped path protects its subtree: `Locked/` and `Locked//` keep `Locked`, `Locked/x.mkv`,
   `Locked/y/z.mkv` and still remove `Lockedness*`; `a/b/` keeps `a/b/c/d.mkv`; `Locked/y/` keeps only `Locked/y/z.mkv`;
   `./` and `/` keep everything. No catalog rule violations recorded.
6. `StartAll` x5 while scans run: one walk per library (2), both complete.

## Observations (non-blocking, no action required for PASS)

1. Per-file debug line `scan could not probe file` logs the prober's error text (`error` attribute). A prober error
   whose text contains the absolute path (fake with `probe <root>/a.mkv: ...`, or timeout/unsupported/other) therefore
   reaches the debug log. The M2-11 contract (and `adapters/ffmpeg/probe.go:83,88`) puts only `filepath.Base` in that
   text, so it is not reachable with the real prober; the finish line (info/warn) is clean in all cases. If the
   guarantee should be independent of the adapter, log only the code for that line.
2. The finish-line `cause` for `catalog_failed` and the file-ID failure includes the catalog/ID generator error text
   (`load file states: <err>`, `save a.mkv: <err>`). A catalog error that contains a root path would be logged; sqlite
   errors do not carry library roots, so this is theoretical.
3. `(*failure).Error` has 0 % coverage (the type is only unwrapped); harmless, total coverage is 97.9 %.
4. Removals run before the probe phase. If the root vanishes after a completed walk (resolve returns `ErrUnavailable`),
   entries the completed walk did not see have already been removed and the failed status shows `Removed: 1`
   (reproduced again: `removed=[id-3]`). Consistent with the delete-only-after-a-complete-walk rule.
5. Windows only was run (no OS-specific code in the package).

## History

- Round 1: PASS (coverage 98.4 %; observations: trailing slash in skipped paths not normalised, removals before probing).
- Round 2: PASS (coverage 97.9 %). The round-1 trailing-slash observation is fixed and verified; new in round 2: shared
  probe semaphore, sanitized finish-line cause, publish-after-logging, in-memory files without video, unresolvable
  files stored as `probe_failed`, `ports.go` contracts with violation-recording fake, ADR notes.

REPORT
status: PASS
task: M2-19
files: docs/reports/M2-19-test.md
tests: go vet ./internal/app/scan/... ok; go test -race -count=5 coverage 97.9 % (>= 90 %); -race -count=20 (also -cpu 1,4 and the service tests) ok; golangci-lint run + fmt --diff 0 issues; archtest ok; go list -deps: only internal/domain{,/library,/media}; go build ./internal/adapters/sqlite/ ok (compile-time check against scan.CatalogWriter holds); scratch probes (3 libraries -> max 2 probes, no root in finish-line logs for walker/resolver/prober/Close errors, Start racing the finish line, no-video memory, trailing-slash skipped paths, round-1 probes) all passed under -race, repeated x10
findings:
- non-blocking: debug line "scan could not probe file" logs the prober's error text; harmless with the real prober (base name only), but a prober error containing an absolute root would reach the debug log
- non-blocking: finish-line cause for catalog_failed / id errors includes the catalog error text (theoretical root leak)
- non-blocking: removals precede probing, so root lost at resolve time yields failed with Removed > 0 (walk had completed)
next: hand to mivo-reviewer; sqlite adapter (M2-15) compiles against the new app/scan; no fixes required
