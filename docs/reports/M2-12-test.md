# M2-12 test report: confined library walker and path resolver

Package under test: `internal/adapters/fs/libraryfs` (doc.go, path.go, walk.go, walk_skip.go, access.go,
identity.go, platform_unix.go, platform_windows.go and tests). Docs touched by the task:
`docs/tasks/M2-12-library-walker.md` (signature line), `docs/adr/0022-...md` (Notes M2-12 r2).
Round 3. Result: **PASS**.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./internal/adapters/fs/...` | PASS | exit 0 (Windows and inside the Linux container) |
| `go test -race -count=1 -v` Windows | PASS | `ok ... libraryfs 2.242s coverage: 88.1% of statements`, 108 `--- PASS`, 8 symlink tests SKIP (reason: `A required privilege is not held by the client`, allowed by the task) |
| Linux, `golang:1.27.3`, root, repo mounted `:ro` | PASS | `coverage: 91.1%`, 88 PASS, 3 SKIP, all `root ignores directory permissions` (`TestWalkReportsAnUnreadableRootAsUnavailable`, `...ReportsARootThatBecomesUnreadableMidWalkAsUnavailable`, `TestWalkLogsAndSkipsUnreadableDirectories`) |
| Linux, `--user 1000:1000` | PASS | `coverage: 93.5%`, 91 PASS, no skips (the three permission tests run) |
| Coverage gate (package >= 85 %) | PASS | 88.1 % Windows / 91.1 % Linux root / 93.5 % Linux uid 1000 (margin is now comfortable; round 2 was 86.2 / 86.6) |
| `golangci-lint run --allow-parallel-runners ./internal/adapters/fs/...` | PASS | `0 issues.` on Windows and with `GOOS=linux` (the `unsafe.Pointer` in platform_windows.go:57 carries a `//nolint:gosec // G103` with a reason) |
| `golangci-lint fmt --diff` | PASS for this task | no diff for `./internal/adapters/fs/...` (Windows and `GOOS=linux`). The unscoped `fmt --diff` exits 1 only because of other tasks' files: `internal/app/library/libraries_test.go`, `internal/app/playback/fakes_test.go`, `internal/adapters/ffmpeg/hlsargs_test.go` (wave 3, ignored per brief) |
| archtest (`go test -count=1 ./internal/archtest/...`) | PASS | `ok ... archtest 1.584s` |
| Layer rule (`go list -deps`) | PASS | project deps of libraryfs on windows and linux: `internal/domain`, `internal/domain/library` only; third party: `golang.org/x/sys/windows` (windows build only) |
| `go mod tidy -diff` | PASS | exit 0, no diff; `git diff -- go.mod go.sum` empty (`golang.org/x/sys v0.49.0` was already a direct requirement) |
| No stray files | PASS | `git status`: only `internal/adapters/fs/libraryfs/` (untracked, expected), the task/ADR edits, this report, and other tasks' files; no `*.out`, `*.orig`, `coverage.out` under the repo. All scratch work lives in the session scratchpad |

## Acceptance criteria

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | `Walk(ctx, root, visit) ([]string, error)` via `os.Root`; only `IsVideoFileName`; skips ignored dirs / hidden; relative slash paths, size, mtime; `ctx.Err()` per entry; unreadable subdirs logged and skipped | MET | signature matches walk.go:73 and the task line; `TestWalk...` suite PASS (ReportsVideoFiles..., Size/ModificationTime, DirectoriesInNameOrder, ContextCancelled, WithACancelledContext, LogsAndSkipsUnreadableDirectories (uid 1000), ...ACLDoesNotLetUsList (Windows)); ctx checked per entry at walk.go:171 and per queued link at walk.go:145 |
| 2 | Missing/unreadable root wraps `domain.ErrUnavailable` | MET | `TestWalkReportsAnUnusableRootAsUnavailable`, `...UnreadableRootAsUnavailable` (uid 1000), `...BecomesUnreadableMidWalk...` (Windows ACL and Linux uid 1000); scratch: root deleted mid-walk on Linux root and uid 1000 gives `library root became unreadable: unavailable: readdirent ...: no such file or directory`; scratch: an identity error on the root also wraps `ErrUnavailable` |
| 3 | Symlinks: inside followed, escaping skipped at info, loops terminate; skip with reason on Windows | MET (N1, N2) | Linux root and uid 1000: `TestWalkFollowsSymlinksThatStayInsideTheRoot`, `...SkipsAndLogsSymlinksThatLeaveTheRoot`, `...TerminatesOnDirectorySymlinkLoops`, `...ListsEveryCrossLinkedDirectoryOnce` PASS; Windows: junction tests PASS, symlink tests SKIP with reason |
| 4 | `Resolve` absolute path after EvalSymlinks + containment, case-insensitive on Windows; escape -> `ErrForbidden`; missing -> `ErrNotFound` | MET | `TestResolve*` and `TestIsWithin*` PASS on all three runs |
| 5 | `Open` via `os.Root` | MET | access.go `libraryRoot.OpenFile`, `TestOpenReturnsTheFileContent` PASS |
| 6 | Tests in `t.TempDir()`: nested, `@eaDir`/`.hidden`, upper-case ext, Unicode, `..` rejected, cancellation | MET | tests above plus `TestResolveAndOpenRejectPathsThatAreNotCanonicalOrLocal` |
| 7 | Gates: lint + layer rules + race + coverage >= 85 % | MET | table above |
| C1 | Carry-over: entry rejected by `ParseRelativePath` logged at warn with root-relative path and skipped | MET | `TestWalkLogsAndSkipsNamesTheCatalogRejects` PASS (Linux); scratch: `M:I-2.mkv` yields exactly one `level=WARN ... path=M:I-2.mkv` |
| C2 | `Resolve`/`Open` check `filepath.IsLocal(FromSlash(rel))` before joining | MET | path.go `nativeRelativePath`, used by both; unchanged since round 2 |
| C3 | Windows: segments ending in `.`/space or containing `:` rejected | MET | `TestCheckWindowsSegments`, `TestResolveAndOpenRejectWindowsDeviceNamesAndAliases`, `TestWalkLogsAndSkipsNamesWindowsCannotOpenSafely` PASS on Windows |

## Round-3 changes verified

| Change | Verdict | Evidence |
|---|---|---|
| Duplicate identity in the REAL tree -> warn + added to skipped; link-reached duplicates stay debug | MET | walk.go:239-251 (`duplicate`); repo tests `TestWalkReportsADirectoryWhoseIdentityCollidesInTheRealTree`, `...RecognisesADuplicateBeforeTheDepthLimit` PASS; scratch below; Linux cross-linked k=6 (link duplicates) gives `skipped=[]` and only debug lines |
| Identity checked before the depth limit | MET | walk.go:218-230 (`admit`); scratch: duplicate at depth 65 is reported as a collision (warn, `identity matches...`), no `depth limit` line |
| Zero / all-ones ids unknown, never recorded, always listed (depth limit still bounds) | MET | scratch: all-zero ids and zero-index-with-device ids list every directory, `skipped` empty, no warn; 100-level tree with unknown ids -> one skipped path with 65 segments, one depth warn, `top.mkv` only; mixed unknown/known ids do not collide. Windows `TestIdentityFromFileIDInfo` / `TestLegacyIdentity` PASS (all-ones -> unknown) |
| Windows identity = 128-bit `FileIdInfo` + volume serial, fallback to the 64-bit index | MET | platform_windows.go:50-94; scratch on NTFS (C:): 9 directories (root + 8 siblings) all `known`, all different, volume serial `0xc8a236c8a23520e`, `high=0x0` (NTFS only fills the low 64 bits of FILE_ID_128), ids identical across two direct identifications and across two complete `Walk`s (`TestAdv3WindowsIDs` PASS). No ReFS/Dev Drive volume on this machine, so the high half was not exercised on a real volume; the conversion is unit-tested |
| Injectable identity function with internal tests | MET | `walk.identify`, `walk_internal_test.go`, `identity_internal_test.go`, `platform_windows_internal_test.go` PASS |
| Link fallback (neither file nor dir) logged at info | MET | walk.go:314; covered on Linux (`link` 83 % on root, 83 % uid 1000) |
| More transient error codes, FIFO test fix | MET | platform_windows.go:38-44; FIFO tests PASS on both Linux runs (no hang) |
| ADR 0022 "Notes (M2-12 r2)" | MET | present; describes one-listing-per-directory, refused links, skipped paths, depth limit, directory identity and the collision rule |

## Adversarial scratch tests (round 3)

Scratch module `...\scratchpad\adv3` (copy of the repo's `go.mod`/`go.sum`, `internal/domain` and the
production files of libraryfs verbatim, including identity.go; the only change is one line
`Listings.Add(1)` in `admit` to count listings). The repo was not touched. Round-2 suite re-run on
Windows, Linux root and Linux uid 1000, plus the new identity tests (internal package, injected `identify`).

Re-run of the round-2 suite: no escape.
- Path-shape and traversal probes (hand-picked plus 5 219 generated): Forbidden or NotFound only; `Open` never returns the secret.
- Linux TOCTOU (directory flipping between real directory and link to outside while Open/Resolve loop, 5 s): root `opens=11518 resolves=8389 leaked=0`, uid 1000 `opens=15035 resolves=9787 leaked=0`; same flip inside Walk loop: 0 leaks (`TestAdvLinuxWalkRace` PASS). Windows junction swap-in: no escape.
- FIFO: `TestAdvLinuxFIFO` PASS; FIFO swapped in during a walk: `err=nil`, `seen=[ok.mkv sub/movie.mkv z2/m.mkv]`, `skipped=[z1]`, no hang.
- Cross-linked k=6 (Linux): every `fN.mkv` exactly once, listings = 9 = physical directories (root, `sub`, d0..d5, link-only `.hid`), 2 ms, `skipped=[]`. Growth k = 2..20: listings = k+2, files = k+2, "already walked" opens = k*k, 0.3 ms to 22 ms (no blow-up). Windows junction k=6: junctions refused as designed, 8 files, 8 listings, 36 info lines, 11 ms.
- Depth 100: Windows and Linux: 66 files, 66 listings, `skipped` = exactly one path with 65 segments, one depth-limit warn.
- Root removal mid-walk: Linux root and uid 1000: `err` wraps `ErrUnavailable`; Windows: root cannot be removed while Walk holds it (sharing violation), scenario skipped; the ACL variant is the repo test (PASS).
- Log hygiene: no absolute path in any log line (checked after every scenario, including the new ones).
- Scratch assertions that fail identically in round 2 and are errors in my own expectations, not defects: `TestAdvLinuxWeirdNames` (quoted backslash in log), `TestAdvLinuxRejectedEntryOneWarn` (`sub/C:odd.mkv` is a valid Linux path), `TestAdvCrossLinkedK6` (expects 8 listings, forgets the `sub` directory: 9 is right) and `TestAdvCrossLinkedGrowth` (expects k+1 listings and k files, the fixture also has root and `sub`: k+2 is right), plus on Windows `TestAdvJunctionCrossLinkedK6` (expects 6 files / 7 listings, fixture has 8 / 8).

New round-3 identity probes (all PASS, Windows and Linux root and uid 1000):

| Probe | Result |
|---|---|
| Colliding real-tree directories `a`, `b`, `c` (`b/deep` below `b`) all report one id | `seen=[a/a.mkv top.mkv]`, `skipped=[b c]`, two `level=WARN ... identity matches another directory ... path=b|c`, no line for `b/deep`, no info/other lines. The files below `b`/`c` are not reported, their directory paths are in `skipped` |
| Subdirectory with the id of the root | `skipped=[sub]`, only `ok.mkv` seen |
| All ids zero; ids with device but zero index | every directory listed, `skipped` empty, no warn |
| Unknown ids interleaved with known unique ids | nothing skipped, 3 of 3 files seen |
| Unknown ids on a 100-level tree | depth limit still stops it: one skipped path (65 segments), one depth warn |
| Duplicate id at depth 65, past the limit | reported as collision (warn `identity matches`, skipped with 65 segments), not as depth-limit skip |
| `identify` fails for a subdirectory / for the root | subdirectory: skipped + warn, rest continues; root: error wraps `ErrUnavailable` |
| Windows NTFS ids of 8 sibling directories | all different, stable across two walks (see table above) |

## Observations (informational, no action required for PASS)

- N1 (unchanged): absolute symlinks that point inside the root and all Windows junctions are refused (info log + skip; Resolve/Open `ErrForbidden`) because `os.Root` cannot follow them; documented in doc.go and ADR 0022. "Links inside the root are followed" holds for relative links only; the reviewer should confirm that reading.
- N2 (unchanged): a directory link to an already walked directory adds nothing; a directory reachable only through a link is reported under the link path; file links under their own path. The scanner (M2-19) must keep catalog entries at or below every returned `skipped` path.
- O1: on an `ErrUnavailable` (or any error) return, `skipped` is nil (documented in the Walk doc comment and the ADR).
- O2: Windows verbatim (`\\?\`) library roots are rejected with `ErrInvalid`; config layer should surface this clearly.
- O3: the Windows high half of FILE_ID_128 was not observable here (NTFS reports 0 there); only the unit tests cover a non-zero high half.
- O4: a real-tree directory that is a bind mount of another one (same device and inode) is correctly reported as a collision (warn + skipped) rather than listed twice; that matches the documented rule.

## Not covered / skipped

- `pnpm`, media corpus, Docker build: not applicable.
- Full repository `go test ./...` and the repository-wide coverage total were not run; wave-3 tasks run in parallel. Domain code is untouched by this task.
- Windows symlink behaviour (needs the symlink privilege) is covered by the Linux runs only.

## History

- Round 1: PASS (Walk returned only `error`; symlink directory aliases reported files under two paths; no per-walk visited set; no FIFO-safe opens). Coverage 89.9 / 90.5 / 92.2 %.
- Round 2: PASS (visited set by directory identity, real tree first, depth limit 64, `Walk` returns skipped paths, FIFO-safe opens, error classification). Coverage 86.2 / 86.6 / 92.2 %.
- Round 3: PASS (real-tree identity collision -> warn + skipped, 128-bit Windows ids, unknown ids never recorded, identity before depth limit, injectable identity, more transient codes, ADR notes). Coverage 88.1 / 91.1 / 93.5 %.

REPORT
status: PASS
task: M2-12
files: docs/reports/M2-12-test.md
tests: go vet; go test -race -count=1 -v on Windows (88.1 %, 108 PASS), Linux root (91.1 %, 88 PASS, 3 root-only skips), Linux uid 1000 (93.5 %, 91 PASS); golangci-lint run 0 issues (windows + GOOS=linux), fmt --diff clean for the package; archtest ok; go mod tidy -diff clean, go.mod/go.sum unchanged; scratch: round-2 adversarial suite (no escape, TOCTOU leaked=0, FIFO no hang, cross-links k=6 linear listings, depth 100, root removal -> ErrUnavailable) plus injected identity collision / zero ids / depth ordering / identity errors and Windows 128-bit id stability on NTFS
findings: no blocking findings; N1/N2 unchanged (refused absolute links and junctions, link-only directories reported under the link path); O1-O4 informational; unscoped `golangci-lint fmt --diff` fails only on other tasks' files (app/library, app/playback, ffmpeg hlsargs_test)
next: send to mivo-reviewer; reviewer to confirm that N1/N2 satisfy "links inside the root are followed"; the owners of the wave-3 files should run `golangci-lint fmt` on them
