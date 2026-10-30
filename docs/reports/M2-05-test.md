# M2-05 Test report: transcode workspace

Status: PASS (round 2)
Date: 2026-10-30
Package: `internal/adapters/fs/transcodedir` (6 production files, 11 test files)

## Gates (round 2)

| Gate | Command | Result |
|---|---|---|
| vet (Windows, and `GOOS=linux`) | `go vet ./internal/adapters/fs/transcodedir/` | clean |
| tests + race + coverage (Windows) | `go test -race -count=3 -coverprofile=... -v ./internal/adapters/fs/transcodedir/` | ok, 94.3 %, 111 PASS, 3 SKIP, 0 FAIL |
| tests + race + coverage (Linux, Docker golang:1.27.3, repo mounted `:ro`, root) | same, `-count=3` | ok, 92.9 %, 96 PASS, 6 SKIP |
| tests + race + coverage (Linux, Docker, `--user 1000:1000`) | same, `-count=3` | ok, 93.6 %, 99 PASS, 3 SKIP |
| lint (Windows and `GOOS=linux`) | `golangci-lint run ./internal/adapters/fs/transcodedir/...` | 0 issues both |
| format | `golangci-lint fmt --diff ...` (both GOOS), `gofmt -l` | no diff |
| layer rules | `go test ./internal/archtest/...` | ok |
| manual layer check | `go list -f '{{.Imports}}'` | `context errors fmt io/fs os path/filepath syscall time uuid` (std only) and `internal/domain` |
| TODO / brand string | grep in package | none |

### Skips (every test runs on at least one platform)

- Windows (3 SKIP lines, all `symlinkOrSkip`, this account lacks the symlink privilege): `TestOpenFileRejectsSymlinksLeavingTheSession`, plus subtests of `TestASessionsSymlinkIsRefused`, `TestRemoveAndPurgeKeepTheTargetOfANestedSymlink` and `TestSessionSymlinksAreRemovedWithoutTheirTargets`. The same scenarios **run on Windows through the junction tests** (below) and with real symlinks on Linux.
- Windows junction tests **ran and passed in every one of the 3 counts** (checked with `-v`): `TestJunctionNamedLikeASessionIsRemovedWithoutItsTarget`, `TestASessionsJunctionIsRefused` (leaving the root, pointing at the root), `TestOpenFileDoesNotFollowAJunctionInsideASession`, `TestRemoveAndPurgeKeepTheTargetOfAJunctionInsideASession`. Real-lock tests also ran: `TestRemoveSucceedsOnceALockedFileIsReleased`, `TestRemoveGivesUpWhenTheContextIsCancelledWhileLocked`, `TestPurgeContinuesAfterALockedSessionAndReportsIt`, `TestIsLockErrorOnWindows`.
- Linux root: `TestRemoveGivesUpWhenTheContextIsCancelledWhileLocked` (Windows-only, `workspace_test.go:274`) and `TestPurgeContinuesAfterAPermanentFailureAndReportsIt` (`purge_unix_test.go:22`, skipped as root).
- Linux uid 1000: only the Windows-only lock test skips. `TestPurgeContinuesAfterAPermanentFailureAndReportsIt` **ran and passed** (and `TestRemoveWithRetry/returns_a_permanent_error_at_once`).

Coverage notes: lowest functions are `ensureSessionsDir` 75.0 % and `openSessions` 81.2 % (error branches of `MkdirAll`/`Mkdir`/`OpenRoot` that need OS-level failures), `sessionEntryNames` 84.6 %. Package total is above the 85 % bar on every platform and run.

## Round-1 findings: verification

| Round-1 finding | Verdict | Evidence |
|---|---|---|
| Windows junction tests in the repo | fixed | `junction_windows_test.go` runs and passes (list above) |
| Nested link inside a session for Remove/Purge | fixed | `TestRemoveAndPurgeKeepTheTargetOfAJunctionInsideASession` (Windows), `TestRemoveAndPurgeKeepTheTargetOfANestedSymlink` (Linux) |
| `sessions` must be a plain directory, also `sessions -> .` | fixed | `requirePlainDirectory` (`workspace.go:215`), `ErrSessionsNotPlainDirectory`; see adversarial run |
| Retry only on lock errors | fixed | `retry.go:43` `policy.retryable`; `lockerror_windows.go` (ERROR_SHARING_VIOLATION 32 and ERROR_ACCESS_DENIED), `lockerror_other.go` always false |
| Unix continue-on-error Purge test | fixed | `purge_unix_test.go`, passes as uid 1000 and skips as root |
| Nits (`maxRemoveWait` as test invariant, mid-loop ctx cancel, max UUID) | fixed | `TestDefaultRetryPolicyIsBounded`, `TestPurgeStopsOnACancelledContext`, `TestRemoveEntriesContinuesAfterAFailure` pass |

## Acceptance criteria

| # | Criterion | Verdict | Evidence |
|---|---|---|---|
| 1 | `New(root)` validates absolute root, `<root>/sessions` created lazily with 0o750 | met | `TestNewRejectsRootsThatAreNotAbsolute`, `TestNewDoesNotTouchTheFileSystem`, `TestCreateMakesTheSessionDirectoryBelowSessions`, `TestCreateMakesAMissingTranscodeRoot`, `TestPurgeCreatesTheSessionsDirectoryOnAFreshRoot` (0o750 asserted on Linux) |
| 2 | `Create` only canonical lowercase UUIDs (`ErrInvalid`), exclusive (`ErrConflict`) | met | `TestCreateRejectsNonCanonicalSessionIDs`, `TestCreateReportsAnExistingSessionAsConflict`, `TestIsSessionID`; `workspace.go:82-85` |
| 3 | `Remove` recursive, bounded ctx-cancellable retry with named constants, missing session no error | met | `TestRemoveDeletesTheSessionRecursively`, `TestRemoveOfAMissingSessionIsNotAnError`, `TestRemoveSucceedsOnceALockedFileIsReleased`, `TestRemoveGivesUpWhenTheContextIsCancelledWhileLocked`, `TestRemoveWithRetry*`; constants `retry.go:12,16` |
| 4 | `Purge` removes only UUID-named entries of `sessions`, decoys untouched, returns count | met | `TestPurgeRemovesOnlySessionDirectories`, `TestPurgeRemovesAFileNamedLikeASession`, junction/symlink tests, adversarial run |
| 5 | `OpenFile` via `os.Root`; `..`, absolute, symlinks fail | met | `TestOpenFileRejectsNamesLeavingTheSession`, `TestOpenFileDoesNotFollowAJunctionInsideASession` (Windows), `TestOpenFileRejectsSymlinksLeavingTheSession` (Linux) |
| 6 | Tests in `t.TempDir()`, coverage >= 85 % | met | 94.3 % / 92.9 % / 93.6 % |
| 7 | Gates: lint, layer rules, `-race`, coverage | met | Gates table |

## Adversarial safety verification (round 2)

Method: the six production files copied into a scratch module outside the repo (domain import replaced by a stub), at `C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\m205scratch` (tests `transcodedir/adv_test.go`, `adv2_test.go`, `adv3_unix_test.go`). Run on Windows with real NTFS junctions and with `-race`, and on Linux in Docker (read-only mount) with real symlinks. Only `t.TempDir()` content was touched; the repo was not modified.

Round-1 checks re-run against the new code, all pass on both platforms: decoys next to and inside `sessions` (non-UUID, uppercase, hyphen-less, braced, `urn_`, prefixed, suffixed, hidden) survive; `Remove`/`Create` with malformed IDs rejected; `sessions` as link leaving the root; UUID-named link inside `sessions`; link inside a session; `OpenFile` escapes (`..`, absolute, backslash, `\\?\`, `a.ts/../../other`).

Changed behaviour vs round 1: a `sessions` link leaving the root is now refused up front by all four operations with `sessions entry is not a plain directory` (Windows reports the junction type as `?---------`, Linux as `L---------`). Purge returns 0, nothing created or deleted outside. The earlier, less clear Windows message (`mkdirat sessions: file exists`) is gone.

New checks:

- **`sessions` junction/symlink to the transcode root itself**, with UUID-named decoy directories directly in the root: `Purge` returned `0` plus `ErrSessionsNotPlainDirectory`, `Remove`, `Create` and `OpenFile` all refused, both decoys survived, `Create` made nothing in the root (Windows junction and Linux symlink). Relative `sessions -> .` symlink (Linux): refused the same way, decoy survived.
- **Links inside a session pointing at a sibling session and at the `sessions` directory itself (cycle)**: `Remove` deleted the session, the link targets (sibling session content) survived (Windows junctions and Linux symlinks).
- **Locked file inside a session (Windows, real open handle)**:
  - Released after 700 ms: `Remove` retried and succeeded after 704.7 ms, session gone.
  - Never released: `Remove` gave up after 1.91 s with `giving up after 20 attempts: ... being used by another process`, file still present. `Purge` with that session locked and a second one free returned `1` plus the joined error for the locked one (continue-on-error).
- **Permanent non-lock error**: `removeWithRetry` with `ERROR_PATH_NOT_FOUND` was called once and returned in under 1 ms. An `Errno(32)` sharing violation was retried 20 times on Windows and exactly once on Linux (`isLockError` always false there).
- **Swap race (TOCTOU between `Lstat` of `sessions` and `OpenRoot`)**: swapping `sessions` with a link to the root while `Purge` ran (Linux 39,915 iterations but only 3 successful swaps; Windows 0 swaps because the rename failed) did not delete the root decoy. This is weak evidence, since the swap rarely landed. The window exists in theory (check-then-open), but exploiting it needs write access to the transcode directory, so I record it as a note and not a finding.

Verdict: the safety claim "never deletes anything outside `<root>/sessions/<canonical-uuid>`" holds on Windows and Linux for all tested vectors, including `sessions -> root`.

## Non-blocking observations

- TOCTOU note above: `requirePlainDirectory` and `root.OpenRoot("sessions")` are two steps. A local attacker who can already write to the transcode root could in theory race them. Not reproducible in my test, low priority.
- `ensureSessionsDir` (75 %) and `openSessions` (81.2 %) error branches are untested, since they need OS-level failures.
- `ERROR_ACCESS_DENIED` is treated as a lock error, so an ACL-denied file on Windows is retried for about 1.9 s per entry before giving up. This is bounded and documented in `lockerror_windows.go`.
- `OpenFile` returns an `*os.File` that the caller must close; intended contract for M2-29.
- A `coverage.out` exists at the repo root. It is git-ignored (not in `git status`) and was not created by this run (coverage went to `%TEMP%`).
- Other wave-1 work is in the tree (`git status` shows many modified API, config, docs and gen-testmedia files); not judged here. The `transcodedir` package and `docs/reports/M2-05-test.md` are untracked as expected.

## History

- Round 1: PASS (Windows 94.3 %, Linux 91.1 %). Reviewer then raised the findings listed above.
- Round 2: PASS (this report). Windows 94.3 %, Linux root 92.9 %, Linux uid 1000 93.6 %; junction tests run and pass; all round-1 findings verified fixed.
