# M2-13 Single server instance per data directory – test report

Status: **PASS** (round 1)

Scope: `internal/platform/instancelock/**` (new), `cmd/mivo/serve.go`, `cmd/mivo/serve_test.go`,
`go.mod`. Wave 2 runs in parallel; only the packages of this task were judged.

## Gates

| Gate | Result | Evidence |
| --- | --- | --- |
| `go vet ./internal/platform/... ./cmd/...` (Windows) | PASS | exit 0 |
| `go test -race -count=1 -coverprofile ./internal/platform/... ./cmd/...` (Windows) | PASS | all packages `ok`; instancelock 73.6 % (Windows skips the POSIX mode code), cmd/mivo 89.0 %, total of the run 95.3 % |
| Same on Linux, `golang:1.27.3`, repo mounted read-only, root | PASS | all `ok`; instancelock 84.9 %, cmd/mivo 89.0 %, total 95.9 % |
| Same on Linux, `--user 1000:1000` | PASS | identical results; symlink, mode-tightening and mode `0600` tests run, none skipped |
| `golangci-lint run --allow-parallel-runners` (Windows and `GOOS=linux`) | PASS | `0 issues.` both |
| `golangci-lint fmt --diff` | PASS | no diff |
| `go mod tidy -diff` (root) and `go -C tools mod tidy -diff` | PASS | no diff; `go.sum` unchanged (`git status` clean for it) |
| `govulncheck ./...` | PASS | `No vulnerabilities found.` |
| `internal/archtest` (layer rules) | PASS | `ok .../internal/archtest` |
| `go vet` `GOOS=darwin`, `GOOS=freebsd` for instancelock; `go build` `GOOS=plan9` | PASS | exit 0 (the `lock_other.go` fallback compiles) |
| Coverage gate (`go-test-coverage --config=.testcoverage.yml`, on the partial profile of this run) | PASS | `Total test coverage: 95.3%`, threshold 70 % |
| Web / E2E / media / Docker build | n/a | task touches none of them |

`internal/domain` is untouched by this task (domain threshold not affected; archtest green).

## Acceptance criteria

1. **`Acquire(dataDir) (*Lock, error)`, exclusive non-blocking OS lock on `<data>/server.lock`,
   `Release()` unlocks and closes, lock dies with the process, file stays – MET.**
   `lock_flock.go:16` `unix.Flock(LOCK_EX|LOCK_NB)`, `lock_windows.go:19` `LockFileEx` with
   `LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY`. Tests
   `TestAcquireCreatesThePrivateLockFileAndLeavesItAfterRelease` (file remains),
   `TestLockOfAKilledProcessIsReleased` (holder subprocess killed, lock free within the 10 s
   budget), `TestReleaseIsIdempotent` PASS on Windows, Linux root and Linux uid 1000.
2. **Second `Acquire` from another process fails with an error wrapping `ErrLocked` that names
   the directory; helper-subprocess test on Windows and Linux – MET.**
   `TestAcquireFailsWhileAnotherProcessHoldsTheLock` (re-exec of `os.Args[0]`, asserts the
   message names the data dir) and `TestAcquireSucceedsAfterTheOtherProcessReleased` PASS on
   Windows and in Docker. The in-process variant `TestAcquireFailsWhileTheDirectoryIsLocked`
   asserts `errors.Is(err, ErrLocked)` and the directory in the message.
3. **`serve` locks after creating the directories, before opening the database, releases last;
   message `another server is already using <data dir>`; test with two starts – MET.**
   `serve.go:84` `paths.Ensure`, `:89` `lockDataDirectory`, `:94` `openAndServe` (database opened
   and closed inside it), `:96` `lock.Release()` after it. `TestRunRefusesASecondServerOnTheSameDataDirectory`
   (`serve_test.go:117`) passes: exit 1, message prefix checked, first server still healthy
   afterwards, next start works after stop.
   Black box with two real binaries (Windows, isolated `MIVO_*` dirs, log level debug):
   - first server: migrations 1 and 2 applied, listening on 127.0.0.1:61781.
   - second start: exit code **1**, stderr
     `mivo: another server is already using <data>: lock <data>\server.lock: locked by another process`.
   - the second process logged only the `starting Mivo` line: 0 lines about migration, database,
     identity; `server.db`, `-wal`, `-shm`, `server.lock` identical in name, size and mtime before
     and after the second start; the first server stayed alive. So the lock is taken before the
     database is opened.
   - `Stop-Process -Force` on the first server, then a new start: running and listening after
     about 120 ms (limit 10 s), same `server_id`.
4. **Lock file mode `0o600` – MET.** `instancelock.go:26`; `assertUnixMode` in the first test
   and `TestAcquireTightensTheModeOfAnExistingLockFile` (0644 file is narrowed) PASS on Linux root
   and uid 1000; Linux black box with the real binary: `stat` shows `mode=600`. Windows has no
   POSIX bits (documented in `restrictMode`).
5. **Gates (lint, layer rules, race tests, coverage, tidy, govulncheck) – MET**, see table.

### Additional checks

- Symlinked `server.lock` (Linux, real binary in Docker): `server.lock -> /tmp/r/outside/evil.lock`
  makes the server exit 1 with `openat server.lock: path escapes from parent`; the outside
  directory stayed empty (nothing created or locked). Unit test
  `TestAcquireDoesNotFollowASymlinkOutOfTheDataDirectory` PASS as root and as uid 1000.
- Unusable directories (missing, regular file, directory at the lock path) return plain errors
  that name the directory and do not wrap `ErrLocked`.
- Docker and network file systems: `/config/data` is a local volume; the package doc states that
  SMB/NFS data directories may not honour the locks (the REPORT of the task must repeat this).

## Observations (no failure)

- The refused-start message repeats the path: `another server is already using <dir>: lock
  <dir>/server.lock: locked by another process`. It satisfies the criterion; shortening is
  optional.
- Windows coverage of `restrictMode` is 18.2 % because the function returns early there; Linux
  covers it (72.7 %). Package total stays above the 70 % gate on both OSes.
- The server logs `starting <name>` before it takes the lock, so a refused second start leaves
  that one info line in its log. Not a defect.
- `serve_test.go` proves "before the database" only by comment and by exit/message; the black
  box above checked it directly.

## Cleanup

Scratch binaries, run directories and the temporary coverage profile were deleted; no `mivo`
test process or Docker container remains. No production code or tests were edited, nothing
staged or committed.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M2-13
files: docs/reports/M2-13-test.md
tests: go vet; go test -race -count=1 ./internal/platform/... ./cmd/... on Windows and Linux (Docker, root and uid 1000) all ok, total coverage 95.3 % / 95.9 %; golangci-lint (windows+linux) 0 issues; fmt, go mod tidy -diff (both modules), govulncheck, archtest clean; darwin/freebsd vet and plan9 build ok; black box with two real binaries verified
findings:
- none blocking; see Observations (duplicated path in the refused-start message, windows-only coverage dip in restrictMode)
- reminder for the implementer REPORT: network file systems (SMB/NFS data dirs) may not honour locks
next: orchestrator can hand M2-13 to mivo-reviewer; the BACKLOG item "sqlite concurrent Migrate -> one process per data dir" can be closed once the review approves
