# M2-39 FFmpeg adapter: prober logs its own stderr, device-path allowlist
Owner: mivo-media
Depends on: M2-11, M2-16, M2-24b, M2-32 (committed)
Files/areas:
- `internal/adapters/ffmpeg/probe.go`, `keyframe.go`, `process.go`, `capabilities.go`,
  `hlsargs.go` (only the call of the device-path check, if its name changes), `doc.go`, and the
  package's tests that call `NewProber`, `DetectCapabilities` or `ErrorOutput`
  (`probe_internal_test.go`, `keyframe_internal_test.go`, `process_internal_test.go`,
  `capabilities_internal_test.go`, `capabilities_test.go`, `producer_internal_test.go`,
  `producer_corpus_test.go`, `probe_corpus_test.go`, `keyframe_corpus_test.go`,
  `hlsargs_corpus_test.go`, `hostile_corpus_test.go`, `probe_network_test.go`) – only the
  lines needed for the new signatures, plus new tests
- `cmd/mivo/serve.go`, `cmd/mivo/wire_tools.go`, `cmd/mivo/wire_tools_test.go`
  (`wire_playback.go` / `wire_library.go` only if a parameter type there has to follow)
- `internal/adapters/corpustest/graph_test.go` (constructor calls only)
- `internal/adapters/http/stream/corpus_test.go` (constructor call only; M2-37 owns every other
  file of that package)
- `docs/tasks/BACKLOG.md` (only the row below)

## Goal
FFmpeg's error output never leaves the adapter: a failed ffprobe run is diagnosable from a
debug log (today the scan cannot see the stderr tail at all, because `app` must not import the
adapter), and no caller can copy the tail into an error or a response. A media path in the
Windows device namespace that does not name a file on a drive or share never reaches FFmpeg.

Closes BACKLOG row (delete it from `docs/tasks/BACKLOG.md` when done, touch no other row):
- `ffmpeg` | "inject `*slog.Logger` into `NewProber` and log the stderr tail at debug inside the
  adapter, then unexport `ErrorOutput`; Windows device-path guard as an allowlist …; doc.go
  mentions `-protocol_whitelist file`; network-test comment wording" (M2-11 review r2)

The decisions are recorded in ADR 0024 ("FFmpeg error output stays inside the adapter") and
ADR 0022 ("Windows device paths are refused by an allowlist"), both in the notes added on
2026-10-31; do not edit the ADRs. Report any deviation.

## Acceptance criteria
- [ ] `NewProber` takes its dependencies in an options struct with the ffprobe path and a
      required `*slog.Logger` and returns `(*Prober, error)`; a missing field wraps
      `domain.ErrInvalid` (same style as `NewProducer`). Test for each missing field.
- [ ] A failed ffprobe run (unsuccessful exit) in `Probe` and in `KeyframeAtOrBefore` logs one
      debug record inside the adapter with the file's base name, the exit code and the stderr
      tail; nothing is logged at info or above for it, and the returned errors are unchanged
      (still no stderr in the error text). Test with a capturing handler and the fake tool in
      `fakeModeFail`.
- [ ] `DetectCapabilities` takes the logger too (third parameter or options struct). A failed
      detection logs exactly one warn record with the message
      `ffmpeg capability detection failed: only direct play available`, the error and the
      stderr tail (when the failure is an exit); `cmd/mivo` no longer logs that failure itself
      (`logCapabilityFailure` removed), so the existing assertion in `wire_tools_test.go`
      (`level=WARN` + that message) still passes and the message appears exactly once. A
      cancelled context still returns its error without a warn record.
- [ ] `ErrorOutput` is unexported (`errorOutput` or removed in favour of `exitError` access);
      `go doc ./internal/adapters/ffmpeg` lists no `ErrorOutput`, and
      `rg -n "ffmpeg\.ErrorOutput" --type go` finds nothing. Tests that asserted the tail through
      it use the internal helper or the captured log record instead.
- [ ] Windows device-path guard is an allowlist: after normalising `/` to `\`, a path starting
      with `\\?\` or `\\.\` (any case) is accepted only when `<letter>:\` or `UNC\` follows the
      prefix; any other device path, and every path starting with `\??\`, is refused with
      `domain.ErrInvalid` by `Probe`, `KeyframeAtOrBefore` and `HLSArguments` (output
      directory). The classification is a pure function testable on every OS (the GOOS check
      stays outside it). Table test with at least: `\\?\C:\m\a.mkv` and `//?/c:/m/a.mkv`
      (accept), `\\?\UNC\nas\share\a.mkv` and `\\.\C:\a.mkv` (accept), `\\.\COM1`,
      `\\.\pipe\x`, `\\?\GLOBALROOT\Device\HarddiskVolume1\a.mkv`, `\\?\Volume{0}\a.mkv`,
      `\??\C:\a.mkv`, `\\?\`, `\\?\C:` (no backslash after the colon) (refuse). The existing
      Windows-only tests (`TestProbeRejectsWindowsDevicePaths`,
      `TestHLSArgumentsRejectsWindowsDeviceOutputDirectories`) stay green.
- [ ] `doc.go` names both input restrictions (`-protocol_whitelist file` and the
      `-format_whitelist`) and no longer says anything that M2-16/M2-17/M2-24 made stale; the
      comments of `probe_network_test.go` describe what the control run proves with both
      whitelists in place (reviewer check).
- [ ] All callers compile with the new signatures: `cmd/mivo` passes its logger;
      `corpustest` and `http/stream/corpus_test.go` pass a discard or test logger.
- [ ] Tests: `go test ./internal/adapters/ffmpeg/... ./internal/adapters/corpustest/...
      ./internal/adapters/http/stream/... ./cmd/mivo/...` green on Windows with FFmpeg on PATH
      (the corpus must be generated); the Linux run of the ffmpeg package in Docker
      (`go test -race ./internal/adapters/ffmpeg/...` via the Docker line of `CLAUDE.md`)
      green.
- [ ] Gates: `go vet ./...`, `go vet -tags webui ./...`, `golangci-lint run`,
      `golangci-lint fmt --diff`, coverage gate (ffmpeg package ≥ its current threshold) pass.

## Notes
- Overlap check with the parallel tasks: M2-37 owns `internal/adapters/http/stream/**` except
  `corpus_test.go`, which this task edits on one line; neither task changes a helper of that
  package. M2-38 (`internal/app/playback/**`) and M2-40 (`app/scan`, `sqlite`, `libraryfs`)
  share no file with this task. The `app/scan.Prober` and `app/playback` keyframe ports keep
  their method signatures, so no `app` file changes. If a further caller of `NewProber` or
  `DetectCapabilities` turns up outside the listed files (e.g. under `scripts/`), stop and
  report it instead of editing a file another task owns.
- A small test helper per test package (`newTestProber(t, path)`) keeps the 20+ call-site
  changes to one line each.
- Debug for probe and keyframe failures because a scan can fail on many files and already
  records each one as `probe_failed`; warn once for capability detection because it is a
  one-time startup failure that changes what the server can do. The tail may contain absolute
  media paths and bytes from the file; it stays in the server log (ADR 0024 Notes M2-24).
- `\\?\Volume{GUID}\` paths are refused on purpose (ADR 0022 note); library roots are drive
  letters or UNC shares.
- The BACKLOG rows "ffmpeg capabilities nits" (`capabilities.go`), "ffmpeg hlsargs", "ffmpeg
  M2-24b review leftovers" and "ffmpeg tests M2-24b review r3 leftovers" are not part of this
  task; leave them in the BACKLOG.
