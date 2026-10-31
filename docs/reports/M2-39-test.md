# M2-39 test report (round 2)

Status: PASS. Worktree `.claude/worktrees/agent-a176c6a4948e84fef` (uncommitted, base 1895eae). No code edited.

## Gates
| Gate | Result |
|---|---|
| `go vet ./...` / `go vet -tags webui ./...` | exit 0 / exit 0 |
| `go test -race -coverprofile=coverage.out ./...` | all packages ok; ffmpeg 95.9 %, corpustest 100 %, http/stream 97.1 % |
| go-test-coverage `.testcoverage.yml` | package threshold PASS, total threshold (70 %) PASS, total 96.2 % |
| `golangci-lint run` / `--build-tags webui` | 0 issues / 0 issues |
| `golangci-lint fmt --diff` | clean |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | clean |
| Docker `go test -race ./internal/adapters/ffmpeg/... ./cmd/mivo/...` (golang:1.27.3, MSYS_NO_PATHCONV=1) | ok ffmpeg 31.5 s, ok cmd/mivo 6.5 s |
| FFmpeg corpus tests (real FFmpeg, no skips) | TestProbeCorpusMatchesManifest, TestProbeHostileFileNames, TestHostileInputsNeverOpenOtherFiles, TestHLSArgumentsKeepsHostileNamesInsideOneArgument PASS |
| domain layer rule | no domain file changed (diff touches only adapters, cmd, docs) |

## Acceptance criteria
- NewProber options: met. `NewProber(ProberOptions{FFprobePath, Logger}) (*Prober, error)`, empty path and nil logger wrap `domain.ErrInvalid` (probe.go). `TestNewProberRequiresEveryOption` PASS.
- Failed ffprobe, one debug record, no stderr in errors: met. `logRunFailure` (probe.go) logs once at debug with `file` (base name), `exit_code`, `stderr`, called in `Probe` and `KeyframeAtOrBefore`; returned errors unchanged. `TestProberLogsFailedRunsAtDebug`, `TestProberLogsNothingForFailuresThatAreNoExit` PASS.
- DetectCapabilities warn: met. Third parameter `*slog.Logger`; one warn record with the exact message, error and stderr; no record when `ctx.Err() != nil`; nil logger gives ErrInvalid. `logCapabilityFailure` removed from cmd/mivo. `TestDetectCapabilitiesLogsTheErrorOutputOfAFailingTool`, `...LogsNoCancellation`, `...RequiresALogger` PASS. `TestRunStartsWithAnFFmpegThatCannotAnswer` asserts `level=WARN` plus the message, exactly once.
- ErrorOutput gone: met. `go doc ./internal/adapters/ffmpeg | grep ErrorOutput` has no match. The only `ErrorOutput` text in Go files is a test function name (`capabilities_internal_test.go:635`). No `ffmpeg.ErrorOutput` use remains. Replaced by unexported `exitAttributes`.
- Device-path allowlist: met. Pure `refusesDevicePath`, with the GOOS check in `isWindowsDevicePath`. `TestRefusesDevicePath` table includes every required row. It also covers `\.\C:\..\COM1`, `\.\UNC\..\pipe\x`, `//./unc/../pipe/x`, `\?\UNC\` (all refuse) and normal drive and UNC paths (accept). Used by Probe, KeyframeAtOrBefore (via `inputArguments`) and `HLSArguments` (hlsargs.go:162). `TestProbeRejectsWindowsDevicePaths` PASS.
- ErrInvalid from DetectCapabilities fails the start: met. `detectCapabilities` in wire_tools.go returns the error for `errors.Is(err, domain.ErrInvalid)`; `TestDetectCapabilitiesFailsTheStartOnAWiringError` PASS.
- doc.go: met. It names `-protocol_whitelist file` and `-format_whitelist`, and the error output policy. The probe_network_test.go comment wording is left to the reviewer.
- Callers compile: met. cmd/mivo passes `app.logger`; corpustest and http/stream/corpus_test.go updated. No further callers of `NewProber` or `DetectCapabilities` turned up (go vet clean on all packages).
- Tests on Windows and Docker: met (see gates).
- Gates: met.
- BACKLOG: met. The `git diff` shows exactly one line removed, the `ffmpeg` row of M2-11 review r2. The other ffmpeg rows remain.

## Degraded start (manual)
Built `./cmd/mivo`, ran with PATH reduced to System32 and a library at `testdata/media`:
- WARN `ffprobe not found: library scans fail until it is installed and the server restarted`
- WARN `ffmpeg not found: only direct play available`
- `GET /healthz` returned 200, and the scan ended with `code=prober_unavailable`. The server starts degraded as before.

## Findings
None blocking. Note: with ffmpeg present but not answering, the adapter logs the warn record once; the Windows-side degraded path is covered by the cmd/mivo test.

## History
- Round 2: PASS (this report).
