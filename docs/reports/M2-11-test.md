# M2-11 test report – FFmpeg adapter (process runner, prober, keyframe lookup)

Status: **PASS** (round 2)
Date: 2026-10-30
Environment: Windows 11, Go 1.27.3, ffprobe 9.0.2 (gyan.dev, on PATH), golangci-lint 2.15.0.
Linux: race test binary built in `golang:1.27.3` (`--network none`), run in the local `mivo:dev`
image (ffprobe 7.1.5-0+deb13u1, user 1000, repo mounted read-only).

## Gates (round 2)

| Gate | Command | Result |
| --- | --- | --- |
| vet | `go vet ./...` (windows), `GOOS=linux go vet ./internal/adapters/ffmpeg/...` | exit 0 / exit 0 |
| tests, race | `go test -race -count=1 -v -coverprofile=<scratchpad> ./internal/adapters/ffmpeg/...` | `ok ... 18.095s coverage: 98.1%`; 160 PASS lines, 0 FAIL, **0 SKIP** |
| network + corpus ran | same `-v` run | `TestProbeNeverFollowsNetworkReferences/{DASH_manifest_named_mkv,HLS_playlist}` PASS (the control run reached the server, so no skip), `TestProbeHostileFileNames/{-dash,-i,Ümlaut,100%}` PASS, corpus tests PASS |
| tests, `-short` | `go test -short -race -count=1 -v ...` | `ok`; skipped are exactly the 4 integration tests (`TestKeyframeAtOrBeforeOnSeekClips`, `TestProbeCorpusMatchesManifest`, `TestProbeHostileFileNames`, `TestProbeNeverFollowsNetworkReferences`) |
| coverage (package) | `go tool cover -func` | 98.1 % (criterion >= 80 %). Not 100 %: `parseSeconds` 85.7 %, `process.wait` / `limitedBuffer.Write` 88.9 %, `isWindowsDevicePath` 75 % (the non-Windows branch; covered on Linux) |
| coverage gate | `go tool -modfile=tools/go.mod go-test-coverage --config=.testcoverage.yml --profile=<ffmpeg profile>` | total 98.1 % (210/214), PASS. Repo-wide total and `internal/domain/...` >= 90 % not re-measured: this task changes no domain code and other packages are in flux |
| lint | `golangci-lint run --allow-parallel-runners ./internal/adapters/ffmpeg/...` (windows and `GOOS=linux`) | `0 issues.` both |
| format | `golangci-lint fmt --diff ./internal/adapters/ffmpeg/...` | no diff, exit 0 |
| layer rules | `go test -count=1 ./internal/archtest/...` | `ok` |
| manual layer check | `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | no match |
| repeated race run | `go test -race -count=20 -run 'Output\|Tail\|Process\|Limited\|Started\|Wait' ./internal/adapters/ffmpeg/` | `ok 59.163s`, no `DATA RACE` |
| Linux, FFmpeg 7.1.5 | race test binary in `mivo:dev`, cwd = package dir, `-test.v -test.count=1` | `PASS`, exit 0; 173 PASS, 0 FAIL, 1 SKIP (`TestProbeRejectsWindowsDevicePaths`, Windows-only by design); `-i.mkv`, `\|pipe.mkv`, `concat:x.mkv`, `what?.mkv` PASS; both network tests PASS (not skipped); no `DATA RACE` |
| stray files | `git status`, repo root listing | no coverage file in the repo root; profiles and binaries are in the scratchpad |

## Acceptance criteria

All criteria from round 1 stay met; the round-2 changes did not break them.

1. **Process runner: met.** `process.go:122` `exec.CommandContext` with an argument slice, no shell;
   `Stdin = nil`; `maxOutputBytes = 8 << 20` (`limitedBuffer`, now with an `atomic.Bool` overflow flag);
   `tailBuffer` 4 KiB cut at a rune boundary (`TestTailBufferKeepsLastBytesAtRuneBoundary`); kill via
   context; `WaitDelay = 5s`; `exitError` text is `"<tool> exited with code N"`, the stderr tail is only
   reachable through the new exported `ffmpeg.ErrorOutput(err)` (`TestErrorOutput`).
2. **`Prober.Probe`: met.** Arguments are now
   `-v error -hide_banner -print_format json -show_format -show_streams -protocol_whitelist file file:<path>`
   (`TestProbeRunsFFprobeWithSafeArguments`, probe_internal_test.go:242); timeout, domain mapping,
   `N/A`/`-99`/`und` handling unchanged. The added `-protocol_whitelist file` is an extension of the
   literal argument list in the criterion and is required by ADR 0022 (see SSRF section).
3. **Fixture unit tests: met.** Now 8 fixtures including `probe-flac-24bit.json`; malformed JSON and
   six more unusable outputs are rejected. `TestTagLookupIsCaseInsensitiveAndDeterministic` covers the
   tag-case tie-break.
4. **`KeyframeAtOrBefore`: met.** Same results as round 1 on both clips; the keyframe arguments also
   carry the whitelist (`keyframe_internal_test.go:180`). The `V:0` selector is documented at
   `keyframe.go:25`; a file without a format start time returns `ErrUnsupported`, documented at
   `keyframe.go:44`.
5. **Corpus integration test: met.** 45 generated cases probed on Windows and Linux, none skipped.
6. **Hostile file names: met.** Committed tests now cover `-dash.mkv`, `-i.mkv`, `Ümlaut ñ 日本.mkv`,
   `100% #hash;x.mkv` everywhere and `concat:x.mkv`, `what?.mkv`, `|pipe.mkv` on Linux.
7. **Gates: met.** See the table.

## Independent SSRF verification

I wrote my own test (`zz_ssrf_qa_test.go`, in the scratchpad, injected with `go test -overlay`, so the
repo is untouched). Each hostile file points at a local TCP listener that counts accepted connections.
A control run (`ffprobe -protocol_whitelist file,http,tcp,...`) shows whether the file can reach the
network at all; then the package's `Probe` and `KeyframeAtOrBefore` run on the same file.

| Hostile file | Control (network allowed) | `Probe` + `KeyframeAtOrBefore` |
| --- | --- | --- |
| DASH MPD named `x.mkv` | 1 connection | 0 connections (ErrUnsupported) |
| DASH MPD named `x.mpd` | 1 connection | 0 |
| HLS media playlist `x.m3u8` | 1 connection | 0 |
| HLS master playlist `m.m3u8` | 1 connection | 0 |
| HLS content in `y.mkv`, `y.mp4` | 0 (not detected by content) | 0 |
| ffconcat with `file http://...` as `.ffconcat` and as `.mkv` | 0 (concat safe mode refuses URLs) | 0 |
| ffconcat with `subfile,,...:http://...` named `d.mkv` | 0 | 0 |
| SDP content named `s.mkv` | 0 | 0 |

Identical results on Windows (FFmpeg 9.0.2) and Linux in `mivo:dev` (FFmpeg 7.1.5). The four cases
with a positive control are the ones where a pass proves the protection. For the others, FFmpeg itself
does not follow the reference (HLS and concat are not detected by content, concat refuses URLs in
safe mode), so no connection arrives either way.

**Mutation check** (done with `go test -overlay` on modified copies of `probe.go`; the repo files were
never touched, and the SHA-256 hashes of all 12 `.go` files in the package are identical before and
after the whole session):

- Mutation B, `allowedProtocols = "file,http,tcp"`: `TestProbeNeverFollowsNetworkReferences` FAILS
  for both subtests ("the server received 1 requests, want none") and
  `TestProbeRunsFFprobeWithSafeArguments` FAILS. The behavioural test does catch a weakened whitelist.
- Mutation A, `inputArguments` without the whitelist: only `TestProbeRunsFFprobeWithSafeArguments`
  FAILS (the recorded argument slice lacks `-protocol_whitelist file`). The network test stays green,
  because FFmpeg's built-in default for `file:` inputs (`file,crypto,data`) already excludes the
  network. This is what the code comment at `probe.go:32` states. So removing the whitelist is
  caught by the argument test and not by the behaviour test (see finding below).

## Round-1 findings, status

- Absolute path in errors: now tested (`TestProbeErrorsNameOnlyTheBaseName`). The "cannot start" error
  still contains the configured ffprobe path (operator-supplied, not a media path); unchanged, accepted.
- `-i.mkv` and `|pipe.mkv`: now committed tests.
- Fake tool is the test binary: unchanged harness note, not a product defect.
- `coverage-m209` in the repo root: gone.

## Findings (none blocking)

- low: removing the whitelist entirely is only caught by the argument-slice assertions, not by the
  network test, because FFmpeg 7.1 and 9.0 already refuse network protocols for `file:` inputs by
  default. Acceptable defence in depth (the explicit list exists so Mivo does not depend on that
  default); a version of FFmpeg that changes the default would be the real exposure, and the
  mutation-B result shows the behaviour test would catch an allowed network.
- low: the 4 integration tests (corpus, keyframes, hostile names, network) skip under `-short` by
  design; CI must run the long mode at least once so the SSRF test does not silently stay skipped.

## History

- Round 1: PASS (all gates, all criteria met; 141 PASS, coverage 98.0 %).
- Round 2: PASS (SSRF whitelist, atomic overflow flag, `ErrorOutput`, new tests; 160 PASS on Windows
  with 0 skips, 173 PASS on Linux, coverage 98.1 %; `-count=20` race run clean).

REPORT
status: PASS
task: M2-11
files: docs/reports/M2-11-test.md
tests: go vet ./... ok; go test -race -count=1 -v ./internal/adapters/ffmpeg/... ok (98.1 %, 160 PASS, 0 SKIP, network and corpus tests ran on ffprobe 9.0.2); -short skips only the 4 integration tests; -race -count=20 on process tests clean; Linux race binary in mivo:dev (ffprobe 7.1.5) ok (173 PASS, 1 Windows-only SKIP); golangci-lint run + fmt --diff clean (windows + linux); archtest ok; coverage gate 98.1 %
findings:
- own SSRF test (10 hostile files incl. DASH/HLS/ffconcat/subfile/SDP, Probe and KeyframeAtOrBefore, Windows and Linux): 0 connections; 4 cases have a positive control (1 connection with network allowed)
- mutation check via go overlay: allowing http,tcp fails TestProbeNeverFollowsNetworkReferences and the argument test; removing the whitelist fails only the argument test (FFmpeg default already blocks network for file: inputs); source hashes unchanged
- low: defence in depth for the whitelist rests on the argument-slice assertion, not on behaviour, with the current FFmpeg versions
- low: cannot-start error still includes the configured ffprobe path (operator-supplied)
next: reviewer can proceed with M2-11 (mivo-reviewer); repo-wide total and domain coverage (>= 90 %) must still be measured once the other wave packages are merged
