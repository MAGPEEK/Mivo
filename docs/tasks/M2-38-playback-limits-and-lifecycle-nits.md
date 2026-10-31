# M2-38 Finished HLS sessions free their FFmpeg slots; lifecycle leftovers
Owner: mivo-backend
Depends on: M2-25 (committed)
Files/areas: `internal/app/playback/**` (`registry.go`, `registry_lifecycle.go`, `reaper.go`,
`service.go` + tests: `registry_internal_test.go`, `reaper_test.go`, `concurrency_test.go`,
`lifecycle_test.go`, `fakes_test.go`, `fixtures_test.go`); `docs/tasks/BACKLOG.md` (only the row
below). No exported API change: `adapters/http/stream`, `restapi` and `cmd/mivo` (edited in
parallel by M2-37 and M2-39) must compile unchanged.

## Goal
A remux or transcode that has finished (or failed) but is still being watched no longer blocks
another viewer from starting a remux or transcode; it only counts toward the overall session
cap. The M2-25 review leftovers are closed: a leaked stream grant is visible in a debug log,
stale comments are fixed, and the reaper has no unlocked read.

Closes BACKLOG row (delete it from `docs/tasks/BACKLOG.md` when done, touch no other row):
- `app/playback` | "ended/failed HLS sessions still count against the FFmpeg-process and
  transcode limits … reflow ADR 0024 line 148" (M2-25 review r2)

Already done by the architect, do not repeat: the ADR 0024 notes ("Finished HLS sessions free
their process slots", "A missed `StreamAccess.Release` pins its session" in "Notes (M2-37 to
M2-41)") and the reflow of the Notes (M2-25) paragraph. If the implementation has to deviate
from those notes, say so in the REPORT.

## Acceptance criteria
- [ ] A visible HLS session in state `ended` or `failed` counts toward `MaxSessions` only, not
      toward the re-encoding limit nor `MaxFFmpegSessions`. Table test on the registry (internal
      test with small limits): with `maxFFmpeg = 1` an ended remux admits a new remux, a failed
      transcode with `maxReencoding = 1` admits a new transcode, a `starting`/`ready` one still
      refuses (`ErrSessionLimitReached`), and with `maxSessions = 1` an ended session still
      refuses any new session.
- [ ] A replacement (`replacesSessionId`) of an ended session is admitted with the same rules
      and the swap keeps working (existing replacement tests stay green).
- [ ] Teardown of a session with stream requests still in flight (`Stop`, replacement, `Close`)
      logs one debug record with the session ID and the number of open requests (key
      `open_requests`); a session without open requests logs no such record. The count is read
      under the registry lock. Test with a capturing `slog` handler.
- [ ] The `sessionEntry` doc comment (registry.go:37, "callers only ever receive copies") says
      what is true: snapshots (`Session`) are copies, while `touch`, `remove`, `removeIdle` and
      `close` hand out the record itself, whose mutable fields are only read or written under
      the lock.
- [ ] `stopIdle` no longer reads `entry.session.LastAccessedAt` outside the lock (reaper.go:51):
      `removeIdle` returns the idle duration computed under the lock (or a snapshot), and the
      log line uses it.
- [ ] `-race` tests: (a) several goroutines call `Release` on copies of one `StreamAccess` at
      once → the in-flight count drops by exactly one and the session becomes reapable;
      (b) `Release` racing `stopIdle`/the reaper and `Stop` on the same session → no data race,
      no negative count, the session is torn down once. Run
      `go test -race -count=20 -run 'Release|Reap|Idle' ./internal/app/playback/...` (cgo) or the
      Docker `-race` line from `CLAUDE.md`, and report which one ran.
- [ ] Tests: `go test ./internal/app/playback/...` green; package coverage does not drop.
- [ ] Gates: `go vet ./...`, `golangci-lint run` (incl. depguard), `golangci-lint fmt --diff`,
      coverage gate pass.

## Notes
- The natural place is `sessionCounts.add`: an existing entry contributes to `reencoding` and
  `ffmpeg` only while its state is neither `StateEnded` nor `StateFailed` (direct-play entries
  never set `reencodes`/`runsFFmpeg`, so they are unaffected). Keep `add` free of lock handling;
  `count` already runs under the lock.
- The window between the production's exit and the `transition` to `ended`/`failed` still
  counts the slot (conservative); do not try to close it.
- For the open-request log, a small registry method (e.g. `openRequests(entry) int`) read under
  the mutex keeps `teardown` lock-free; `endAccess` may still decrement the count of a removed
  record concurrently, which is why the read must be locked.
- Do not add a per-access timeout or any other safety net for a missed `Release`; ADR 0024
  rules it out.
- Keep functions ≤ 50 lines and cognitive complexity ≤ 15; the existing concurrency tests show
  how the fakes and the injectable clock/`after` are used.
