# M2-09 test report (domain playback decision engine)

Status: **PASS** (round 2)
Date: 2026-10-30
Scope: `internal/domain/playback/**` and `internal/domain/playback/playbacktest/**` (untracked), plus the "Notes (M2-09)" section of `docs/adr/0023-playback-decision-engine-and-client-profiles.md`.
Other wave-2 packages were not judged.

## Round-2 changes under test

- An unknown bit depth now fails closed against a `MaxBitDepth` limit (`video_bit_depth_not_supported`, then an 8-bit transcode). An unknown profile still passes.
- The `HLSMuxing` check moved after per-stream planning, so client limits (`no_compatible_format`) win over server limits (`transcode_unavailable`).
- `Policy.Validate` is exported. A negative or odd height is `ErrInvalid`; 0 means no cap (confirmed by the orchestrator).
- The transcode target is always even. `StreamDecision.ScaleRequired` is new; `Downscaled` now only drives the warning.
- `validateText` rejects control characters.
- ADR 0023 has a "Notes (M2-09)" section.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/domain/...` | exit 0, no output |
| tests + race | `go test -race -count=1 "-coverprofile=<temp>" ./internal/domain/...` | all `ok` (domain, library, media, playback, playbacktest, system) |
| coverage | `go tool cover "-func=<temp>"` | `playback` 100.0 %, `playbacktest` 100.0 %, library, media and system 100.0 %. A filter for functions below 100.0 % printed nothing (threshold 90 %). |
| lint | `golangci-lint run --allow-parallel-runners ./internal/domain/playback/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/domain/playback/...` | exit 0, empty diff |
| archtest | `go test -count=1 ./internal/archtest/...` | `ok` |
| pure-domain imports | `go list -f '{{.ImportPath}}: {{.Imports}}'` | `playback` imports only `errors fmt slices strings time unicode unicode/utf8`, `internal/domain`, `internal/domain/media`. `playbacktest` imports only `media` and `playback`. |
| forbidden deps | `go list -deps ./internal/domain/playback/...` filtered for `database/sql\|net/http\|os/exec\|internal/adapters\|internal/app` | no match |
| total coverage (>= 70 %) | not run | out of scope for this package-level run; both packages are at 100 % |

The coverage profile went to `%TEMP%` (quoted `-coverprofile`) and was deleted. `git status` shows no coverage, probe or `zz_` file.

## Acceptance criteria

All eight criteria remain met. The round-2 changes touch these:

1. **Types** - met. `ServerCapabilities`, `Request`, `Policy` (with the new `Validate`) and `StreamDecision` (with `ScaleRequired`) are in `input.go`, `plan.go` and `capabilities.go`.
2. **`Decide` follows ADR 0023 in order** - met. The order is `validate`, `selectStreams`, `directPlayReasons`, `hlsPlan` (decide.go:21-46). `TestDecideFormatMatrix` and the other `Test*` functions are green.
3. **Errors** - met. The sentinels wrap `domain.ErrUnsupported` (probe rows 5/5b/5c/5g show `unsupported=true`). An unknown audio index is `ErrInvalid`.
4. **Transcode targets** - met. H.264 High 8-bit and AAC stereo. A downscale happens only above the cap. The width is rounded to an even number. `hdr_not_tone_mapped` is emitted only on a transcode.
5. **`playbacktest`** - met, unchanged, 100 %.
6. **Matrix test** - met. It gained two rows, "mp4 with H.264 of unknown bit depth" and "webm with VP9 of unknown bit depth", plus "mkv with 4K HDR10 HEVC". Each row runs against 2 profiles, 2 capability sets and 2 deliveries.
7. **Coverage >= 90 %, no mocks** - met (100 % / 100 %).
8. **Gates** - met (table above).

## Hand derivation of the changed behaviour

I ran these through `Decide` as a probe test. The probe was not added to the repo: it was injected with `go test -overlay` from the scratchpad. I compared each output to what ADR 0023 plus its Notes prescribes. The profile is `ChromiumDesktop()` with `FullCapabilities()` and cap 1080 unless noted.

| # | Input | ADR expectation | Observed | Match |
|---|---|---|---|---|
| 1 | mp4 H.264 High, depth unknown (no depth, no pixel format), AAC | h264 limit `maxBitDepth` 8, depth unknown, so it fails closed. The 8-bit target passes. Transcode video, copy AAC. | transcode, reasons `[video_bit_depth_not_supported]`, video `h264->h264` 320x240 `libx264`, audio copy | yes |
| 1b | mp4 H.264, 8-bit, profile unknown | an unknown profile passes | direct_play | yes |
| 2 | webm VP9, depth unknown, Opus | vp9 `maxBitDepth` 10, so it fails closed | transcode `[video_bit_depth_not_supported]`, `vp9->h264`, Opus copy | yes |
| 3 | webm VP9 720x405, depth unknown | 405 <= 1080, so no downscale. The odd height is rounded down, so 720x404. A scale filter is needed. No warning. | `720x404 down=false scale=true`, warnings `[]` | yes |
| 3b | the same at 721x405 | width rounded down too | `720x404 scale=true`, no warning | yes |
| 4 | mkv 4K HDR10 HEVC + E-AC-3 | container, video-codec and audio-codec reasons. Video is transcoded to 1080p (OD-5 B), audio to AAC stereo, with HDR and downscale warnings. | `1920x1080 down=true scale=true`, audio `eac3->aac` ch 2, warnings `[hdr_not_tone_mapped, video_downscaled]` | yes |
| 4b | the same without a cap | keeps 3840x2160 and drops the downscale warning | `3840x2160 down=false scale=false`, warnings `[hdr_not_tone_mapped]` | yes |
| 5 | zero capabilities, HLS video list `vp9 av1`, HEVC source | the client cannot play the H.264 target, so `no_compatible_format` before `transcode_unavailable` | `no_compatible_format` (and not `transcode_unavailable`) | yes |
| 5b | the same with an H.264 source | the copy is impossible, the target is impossible | `no_compatible_format` | yes |
| 5c | zero capabilities, HLS audio list `opus`, AC-3 source | the client plays no AAC target | `no_compatible_format` | yes |
| 5d | zero capabilities, plain remux (mkv H.264/AAC) | the client is fine, the server has no HLS muxer | `transcode_unavailable` ("server cannot write HLS") | yes |
| 5e | zero capabilities, HEVC (client plays H.264) | `transcode_unavailable` | `transcode_unavailable` | yes |
| 5f | zero capabilities, mp4 H.264/AAC | direct play needs no FFmpeg | direct_play | yes |
| 5g | muxer present, no encoder, HEVC | `transcode_unavailable` ("no h264 encoder") | `transcode_unavailable` | yes |
| 6 | profile name `"a\nb"` | `ErrInvalid` | `invalid=true`: "name contains a control character" | yes |
| 6a | names with CR, ESC `\x1b[2J`, DEL, C1 `\u0085`, TAB | `ErrInvalid` (ADR Notes: CR, LF, ESC, DEL, C1) | all `invalid=true` | yes |
| 6b | `"ok name"` and `"Jörg ✓"` | valid | direct_play | yes |
| 6c | codec-limit profile `"Main\n"` | `ErrInvalid` | `invalid=true` ("codec limit 0: profile contains a control character") | yes |
| 7 | `Policy` height -2, -1, 1, 1081 | `ErrInvalid`, and `Policy.Validate()` returns the same error | all `invalid=true` | yes |
| 7b | `Policy` height 0, 2, 1080 | valid (0 means no cap) | `Validate() == nil`, decision runs | yes |
| 8 | mkv HEVC 1920x1081, cap 1080 | width = nearest even number of 1920*1080/1081 = 1918.2 | `1918x1080 down=true scale=true`, warning `video_downscaled` | yes |

ADR 0023 and the code agree on every Notes bullet: profile validation (unknown keys tolerated, control characters rejected), merged duplicate direct-play keys, codec limits (video only, case-insensitive profile, unknown profile passes, unknown depth fails closed), failure precedence, the policy (exported `Validate`, 0 = no cap, guarded again in `Decide`), the request (`ErrInvalid` for a bad start position or audio index), no-audio-no-video giving plain `ErrUnsupported`, the even transcode size with `ScaleRequired`, and the internal helpers. No deviations. The ADR file is unchanged by me (SHA-256 `801CE660...2699`).

## Mutation check

I used `go test -overlay` with mutated copies of the files in the scratchpad. The repo files were never touched. I hashed the 16 files of `internal/domain/playback/**` (SHA-256) before and after: **identical**. `go test` of both packages is green afterwards.

| # | Mutation | Result |
|---|---|---|
| A | fail-closed rule removed: `allowsBitDepth` returns `depth <= limit.MaxBitDepth` (`checks.go:147`) | killed: `TestDecideCodecLimits`, `TestDecideFormatMatrix` FAIL |
| B | old precedence: `HLSMuxing` check as the first statement of `hlsPlan` | killed: `TestDecideServerAndClientLimits/a_client_limit_wins_over_missing_FFmpeg` FAIL |
| B2 | the same check just before per-stream planning (after the delivery check) | killed: the same test |
| C | an unknown profile fails (`profile == ""` shortcut removed) | killed: `TestDecideCodecLimits` FAIL |
| D | `scaleRequired` always false | killed: `TestDecideTranscodeSize`, `TestDecideTranscodesOddSizedVP9ToEvenSize` FAIL |
| E | control-character check disabled in `validateText` | killed: `TestClientProfileValidateRejectsInvalidProfiles` FAIL |
| F | odd `MaxTranscodeHeight` accepted | killed: `TestDecideRejectsInvalidInput`, `TestPolicyValidate` FAIL |

Mutation B first failed to build: my `sed` hit two `plan := Plan{` lines and a duplicated block broke the build. I redid it with an exact insertion point, and B and B2 are the valid results.

## Findings

- None blocking.
- Info: `TAB` also counts as a control character (`unicode.IsControl`) and is rejected in profile text. This is consistent with the stated goal (no forged log lines) and the tests cover it.
- Info: `maxBitDepth` 0 is accepted by `Validate` ("unlimited") although the schema has `minimum: 1`. This is by design (round 1, unchanged).
- Info: in `matrix_test.go`, `expectedFor` derives the delivery-none and no-encoder variants with the engine's own rule. The mse/full rows are literal. The new unit tests for precedence are independent and caught B and B2.

## History

- Round 1 (2026-10-30): PASS. Gates green, 100 % coverage for both packages, all criteria met, reference profiles equal to ADR 0025, corpus rows complete, `ClientProfile.Validate` agrees with the OpenAPI schema (minor: `maxBitDepth` 0), `AudioByIndex` miss is `ErrInvalid`. Mutations (`audioIsDefault` always true, upscaling to the cap) were killed.
- Round 2 (2026-10-30): PASS. Same gates green. New behaviour hand-derived (22 probe rows) and consistent with ADR 0023 including the Notes. Seven mutations killed, hashes identical after.
