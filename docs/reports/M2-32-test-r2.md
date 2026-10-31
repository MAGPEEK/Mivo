# M2-32 test report, round 2

Status: **PASS**

Setup: clean git worktree at HEAD a2d459e, plus exactly the 62 files of `m2-32-files.txt`
(50 modified, 12 new). The files were staged in the worktree (`git add -A`) so that
`git diff --exit-code` after each generator shows only drift. Worktree removed afterwards
(needed the `\\?\` prefix because of long paths) and pruned.

## Gates

| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go vet -tags webui ./...` (after `pnpm -C web build`) | clean |
| `GOOS=linux GOARCH=arm go vet ./...` | clean |
| `go test -race -coverprofile=coverage.out ./...` (native, cgo via LLVM-MinGW) | 36 packages ok, 0 failures, no skips in `cmd/mivo` |
| `go-test-coverage --config=.testcoverage.yml` | total 96.5 % (5334/5527) >= 70 %; every `internal/domain/...` package >= 90 %: PASS |
| Docker `go test -race ./...` (golang:1.27.3) | all ok, rc=0 |
| `golangci-lint run` / `--build-tags webui` | 0 issues / 0 issues |
| `golangci-lint fmt --diff` | no diff |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | no diff |
| `go generate ./...` | no drift |
| sqlc generate (Docker) | no drift |
| `pnpm -C web gen` | no drift |
| `pnpm -C web install` / `lint` | ok / clean (ESLint, depcruise, Prettier, tsc) |
| `pnpm -C web test` | 82 files, 1203 tests passed, thresholds met |
| `pnpm -C web build` | ok |
| `pnpm -C web e2e` | 22 passed |
| `docker build -f packaging/docker/Dockerfile` | ok (rc=0) |
| `govulncheck ./...` | No vulnerabilities found |
| Layer rule (`go list -deps ./internal/domain/...`) | no `database/sql`, `net/http`, `os/exec`, adapters or app |

## Walking skeleton (built `-tags webui` binary, port 18461, temp config/data/cache/transcode)

Corpus generated with `go run ./scripts/gen-testmedia` (45 files). `MIVO_LIBRARIES=<worktree>/testdata/media`.

- Startup log in the documented order: migrations, server identity, tool found (ffprobe,
  ffmpeg), "ffmpeg ready for direct play, remux and transcode" (version, libx264, aac),
  libraries synced, workspace purged, http listening `lan_only=true`, startup scan
  `added=28 failed=0`.
- `GET /api/v1/libraries` 200 (1 library, itemCount 28); `POST .../scan` 202, finishes, 28 items listed.
- direct play (`h264_aac_faststart.mp4`): session 201 `mode=direct_play delivery=file`;
  Range `bytes=0-99` -> 206 `bytes 0-99/100954`; HEAD 200; DELETE 204.
- remux (`h264_aac.mkv`): 201 `mode=remux` HLS, playlist 200 `application/vnd.apple.mpegurl`,
  `seg-00000.m4s` 200 (99513 bytes); DELETE 204.
- transcode (`mpeg2_mp2.ts`, profile h264/aac only): 201 `mode=transcode`, playlist 200,
  `seg-00000.m4s` 200 (147040 bytes); DELETE 204.
- Session DELETEs are logged at info ("http request ... status=204"); stream routes are quiet.
- No browser (Chrome/Edge) was driven; the check ran through the API/HLS, as instructed. The
  Playwright e2e run covers the UI with mocked sessions.

## Round-2 review fixes

- Shutdown bounds: `ShutdownTimeout` 10 s (`internal/adapters/http/server/server.go:23`) +
  `playbackCloseTimeout` 5 s (`cmd/mivo/wire_playback.go:30`) + `scanCloseTimeout` 2 s
  (`cmd/mivo/wire_library.go:22`) + `databaseCloseReserve` 1 s (`cmd/mivo/serve.go:45`) = 18 s
  = `shutdownBudget` < 20 s; guarded by `TestShutdownStepsFitTheBudget` (passes).
  Live: Docker image, library `/media`, transcode of `mpeg2_mp2_long.ts` running (ffmpeg
  process visible in `docker top`, session dir present), `docker stop` (`--stop-timeout 20`)
  returned in 1.1 s, exit code 0, log: "http server shutting down", "playback sessions
  stopped sessions=1", "library scans stopped", "Mivo stopped".
- Path escape on the direct-play file route answers 422 `unsupported`, not 403
  (`internal/adapters/http/stream/direct.go` `openFailureProblem`;
  `TestDirectFileOutsideItsLibraryAnswersUnsupported` passes).
- Missing tool is not looked up again per run: `unavailableToolPath` hands the adapters a
  never-created path (`cmd/mivo/wire_tools.go:114-120`); `TestMissingToolIsNeverLookedUpAgain` passes.
- `withCleanupError` is used in `cmd/mivo/wire_playback.go:93` (and in `serve.go`).
- HEAD HLS spec (`api/openapi.yaml`, `headPlaybackHlsFile`) lists 200, 206, 416, 503 with
  `Retry-After`, the `Range` parameter, and the first-segment wait.
- `GOOS=linux GOARCH=arm go vet ./...` passes.

## Acceptance criteria

- [x] Startup order: confirmed by the startup log above and `TestRunLocksBeforeOpeningTheDatabase`,
      `TestRunRefusesATranscodePathWithATemplateMarker`, `TestRunRefusesASessionsEntryThatIsNoDirectory`;
      capability line logged. LAN guard wired: `AllowPublicClients: !LANOnly`
      (`cmd/mivo/wire_http.go:86`), quiet patterns `stream.QuietPatterns()`; interface prefixes
      left nil = host interfaces (documented in `serverOptions`).
- [x] Missing ffprobe/ffmpeg: `TestRunStartsWithoutFFmpegTools`, `TestRunStartsWithAnFFmpegThatCannotAnswer`,
      `TestRunWithoutFFmpegRefusesRemux` pass.
- [x] Shutdown order and bounds: see above (live and unit).
- [x] `restapi` rejects nil services with `ErrInvalid` (`options.go:44-64`,
      `TestNewHandlerRejectsInvalidOptions`); no `not_implemented` anywhere in Go or YAML.
- [x] `cmd/mivo` corpus test `TestRunScansAndPlaysTheCorpus` ran (not skipped) and passes.
- [x] Manual check: performed via API as above; `docs/development.md` documents the
      `MIVO_LIBRARIES` / isolated-dir procedure.
- [x] Gates: all green (table above).
- [x] Orchestrator notes: LAN guard test (`TestRunGuardsTheLANByDefault`), session DELETE at info
      (`TestRunLogsSessionStopsAtInfo`), 403/422 spec text in the operation description,
      `contract_values.go` added, `RootAvailable` in `libraryfs/availability.go`.

## Findings

None blocking. Note for the orchestrator: the worktree-based `git diff --exit-code` check
needs the 62 files staged first, otherwise it reports the copied changes as drift.

## History

- Round 2 (this report): PASS. No round-1 report file for M2-32 exists in `docs/reports/`.
