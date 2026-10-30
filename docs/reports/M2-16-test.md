# M2-16 test report – FFmpeg adapter: detect server capabilities

Status: **PASS** (round 1)
Date: 2026-10-30
Environment: Windows 11, Go 1.27.3, FFmpeg 9.0.2 (gyan.dev, on PATH), golangci-lint 2.15.0.
Linux: race test binary built in `golang:1.27.3` (`--network none`, repo mounted `:ro`), run in the local
`mivo:dev` image (FFmpeg 7.1.5-0+deb13u1, `--network none`, repo `:ro`). Tested together with M2-17
(same package); the results below are for the shared run.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| vet | `go vet ./internal/adapters/ffmpeg/... ./internal/domain/...` | exit 0. `go vet ./...` fails in `internal/app/scan` (`probe.go:147`, `run.go:45` ...: `not enough arguments in call to fail`): uncommitted work of another task, not part of M2-16/17 |
| tests, race | `go test -race -count=1 -v -coverprofile=<scratchpad> ./internal/adapters/ffmpeg/...` | `ok ... 26.226s coverage: 99.0%`; 271 PASS, 0 FAIL, **0 SKIP** (integration tests ran: `TestDetectCapabilitiesOnLocalFFmpeg` PASS 0.15 s, `TestDetectCapabilitiesThroughAProcess` PASS incl. failing and hanging tool) |
| tests, `-short` | `go test -race -count=1 -short -v ...` | `ok`; `TestDetectCapabilitiesOnLocalFFmpeg` SKIP ("skipping FFmpeg integration test in -short mode"), parser/fake tests run |
| repeat | `go test -race -count=5 -run 'Capab\|HLSArg\|DetectCap'` | `ok 28.046s`, no flake, no `DATA RACE` |
| coverage (files) | `go tool cover -func` | every function in `capabilities.go` 100.0 % |
| coverage gate | `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml --profile=<ffmpeg profile>` | package 99.0 % (400/404), total threshold 70 % satisfied, exit 0 |
| coverage, repo | `go test -count=1 -coverprofile ./...` | covered packages total 82.4 % (>= 70 %); all `internal/domain/...` packages 100.0 % (>= 90 %). Not a clean repo-wide number: `cmd/mivo`, `internal/adapters/sqlite`, `internal/app/playback` fail to build and `internal/app/scan` fails a test - other tasks in flight, unrelated to the ffmpeg package |
| lint | `golangci-lint run --allow-parallel-runners ./internal/adapters/ffmpeg/...` (windows and `GOOS=linux`) | `0 issues.` both |
| format | `golangci-lint fmt --diff ./internal/adapters/ffmpeg/...` (windows and `GOOS=linux`) | no diff, exit 0 |
| layer rules | `go test -count=1 ./internal/archtest/...` | `ok` |
| manual layer check | `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | no match; the ffmpeg package imports only `internal/domain`, `domain/media`, `domain/playback` |
| Linux, FFmpeg 7.1.5 | race test binary in `mivo:dev`, cwd = package dir, `-test.v -test.count=1` | exit 0; 272 PASS, 0 FAIL, 2 SKIP (`TestHLSArgumentsRejectsWindowsDeviceOutputDirectories`, `TestProbeRejectsWindowsDevicePaths`, both Windows-only by design); `TestDetectCapabilitiesOnLocalFFmpeg` PASS (0.21 s, not skipped); no `DATA RACE` |
| stray files | `git status`, repo root | only the expected new files; `mivo.exe` in the root is git-ignored (`*.exe`) and predates this run; all my probes, overlays, fakes and binaries are in the scratchpad |

## Acceptance criteria

1. **`DetectCapabilities(ctx, ffmpegPath)`: met.** `capabilities.go:75` runs `-hide_banner -version`,
   `-encoders`, `-muxers` through `tool.output` (M2-11 runner) under `capabilityDetectionTimeout` (30 s) and fills
   version, `HLSMuxing` (needs both `hls` and `mp4` muxers with the `E` flag), `H264Encoder` (libx264, kind `V`,
   not experimental) and `AACEncoder` (aac, kind `A`). `TestDetectCapabilitiesRunsTheThreeQueries` and
   `TestDetectCapabilitiesThroughAProcess/arguments_reach_the_process_verbatim` PASS.
   Independent check (overlay test, no repo edit) on the real tools:
   - Windows: `caps={FFmpegVersion:9.0.2-full_build-www.gyan.dev HLSMuxing:true H264Encoder:libx264 AACEncoder:aac} err=<nil>`
   - mivo:dev: `caps={FFmpegVersion:7.1.5-0+deb13u1 HLSMuxing:true H264Encoder:libx264 AACEncoder:aac} err=<nil>`
2. **Missing executable -> zero capabilities + `domain.ErrUnavailable`: met.** Independent check with the
   compiled fakes (`fake-garbage`, `fake-exit1`, `fake-hang`) and a path that does not exist, all via
   `DetectCapabilities` with a background context; every case returned the zero `ServerCapabilities` and
   `errors.Is(err, domain.ErrUnavailable) == true`:

   | Tool | Time | Error |
   | --- | --- | --- |
   | does-not-exist | 7 ms | `ffmpeg -version: start ffmpeg: unavailable: fork/exec ...: The system cannot find the file specified.` |
   | prints garbage, exit 0 | 217 ms | `unavailable: parse ffmpeg -version: no "ffmpeg version" line in the output` |
   | exits 1 | 178 ms | `ffmpeg -version: unavailable: ffmpeg exited with code 1` |
   | hangs | 30.004 s | `ffmpeg -version: unavailable: run ffmpeg: context deadline exceeded` (stopped by the detection timeout) |

   Caller cancellation (cancel after 500 ms against the hanging tool): returns after 504 ms with
   `context canceled`, `errors.Is(err, context.Canceled) == true`, **not** `ErrUnavailable`, zero capabilities. An
   already-cancelled context behaves the same (`start ffmpeg: context canceled`). Matches the doc comment.
3. **Parser tests on FFmpeg 9.0.2 and 7.1.x fixtures: met.** `TestCapabilityOutputsParseFixtures/{9.0.2,7.1.5}`
   PASS. The 9.0.2 `encoders`/`muxers` fixtures equal `scripts/gen-testmedia/testdata/ffmpeg-9.0.2-*.txt` (body
   identical, one `#` header line added) and equal the live `ffmpeg -hide_banner -encoders|-muxers` output. The
   7.1.5 fixtures equal the live output of `docker run --rm --entrypoint ffmpeg mivo:dev -hide_banner -encoders|-muxers`,
   and the capture command is in each fixture header (`TestCapabilityFixturesHaveProvenance` PASS). The
   name-column matching is covered by `TestParseCapabilitiesMatchesTheNameColumn` (prefixes such as libx264rgb,
   wrong kind, experimental, no `E` flag, aliases, CRLF, legend lines before the separator).
4. **Integration test against the local FFmpeg: met.** `TestDetectCapabilitiesOnLocalFFmpeg` PASS on Windows
   (9.0.2) and in `mivo:dev` (7.1.5); skips with `-short` / without FFmpeg.
5. **Gates: met.** lint, layer rules, `go test -race ./internal/adapters/ffmpeg/...` and the coverage gate pass
   (table above).

## Findings

None blocking.

- Observation (no action): `go vet ./...` and the repo-wide test run currently fail in packages outside this
  task (`internal/app/scan`, `internal/app/playback`, `internal/adapters/sqlite`, `cmd/mivo`), which have
  uncommitted work in progress. The orchestrator must re-run the repo-wide gates once those tasks land.

## History

- Round 1 (2026-10-30): PASS.
