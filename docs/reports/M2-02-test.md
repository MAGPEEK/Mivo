# M2-02 Test Report - Domain: technical description of media files

Status: **PASS** (round 1)
Date: 2026-10-30
Toolchain: go1.27.3 windows/amd64, golangci-lint 2.15.0, ffprobe 9.0.2 (gyan full build)
Scope: `internal/domain/media/**` (17 new files). Wave 1 siblings (M2-04 config, M2-06 gen-testmedia,
restapi work) are outside this task and not judged.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet | `go vet ./internal/domain/...` | exit 0, no output |
| test + race | `go test -race -count=1 -coverprofile=... ./internal/domain/media/` | `ok ... coverage: 100.0% of statements` |
| domain tree | `go test -race -count=1 -cover ./internal/domain/...` | all `ok`; domain/library 100 %, domain/media 100 %, domain/system 100 % |
| coverage gate | `go tool "-modfile=tools/go.mod" go-test-coverage --config=.testcoverage.yml --profile=<media profile>` | `Total coverage threshold (70%) satisfied: PASS`, `Total test coverage: 100% (214/214)` (profile limited to this package because other packages do not build right now) |
| lint | `golangci-lint run ./internal/domain/media/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/domain/media/...` | exit 0, no diff |
| layer rule (archtest) | `go test -count=1 ./internal/archtest/...` | `ok` |
| imports | `go list -f '{{join .Imports "\n"}}' ./internal/domain/media/` | `fmt, internal/domain, math, slices, strconv, strings, time` only |
| deps scan | `go list -deps ./internal/domain/media/ \| Select-String 'database/sql\|net/http\|os/exec\|adapters\|internal/app'` | no match |
| TODO scan | `grep -rn "TODO\|FIXME" internal/domain/media` | none |
| web / docker | skipped | task does not touch them |

Every function of the package is at 100.0 % (`go tool cover -func`).

### Out-of-scope observations (not judged, for the orchestrator)

- `golangci-lint run` (whole repo): typecheck error in `scripts/gen-testmedia` test files
  (`undefined: seekClipDuration`, `seekCaseSuffix`, `seekKeyframeInterval`) - M2-06 in progress.
  The run still printed `0 issues.` for everything that type-checks.
- `go build ./...`: `internal/adapters/http/restapi/api.go:15:2: "github.com/magpeek/mivo/internal/domain" imported and not used` - parallel restapi work.
- `go-test-coverage` with the default `coverage.out` was not run repo-wide for these reasons.

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Types `Container`, `StreamKind`, `Stream` (all listed fields), `Description`, `File` | Met | `container.go:11`, `stream.go:11,38-82`, `description.go:11`, `file.go:30`. Stream has Index, Kind, Codec, Profile, Level, PixelFormat, BitDepth, Width, Height, FrameRate, ColorPrimaries/Transfer/Space, HDR, Channels, ChannelLayout, SampleRate, Language, Title, Default, Forced, AttachedPicture, BitRate (+ extra CodecTag). Description has Container, FormatName, Duration, BitRate, Streams. File has ID, ItemID, LibraryID, RelativePath, Size, ModifiedAt, Description, ProbeError. |
| 2 | `IdentifyContainer` table test covers all corpus containers incl. `.mts`, `.rmvb`, unknown -> `ContainerUnknown` | Met | `container_test.go:37` (`.mts` -> m2ts), `:45` (`.rmvb` -> rm), `:123` (unknown), `:78` (`.3g2`). Real-file cross-check below confirms all 22 corpus videos. |
| 3 | `DirectPlayKey` / `MIMEType` implement the ADR 0023 table | Met | `directplay.go:22-67` compared line by line with ADR table: mp4/m4v -> mp4 `video/mp4`; mov `video/quicktime`; webm `video/webm`; mkv `video/x-matroska`; ogg `video/ogg`; mpegts/m2ts -> mpegts `video/mp2t`; 3gp `video/3gpp`; avi, flv, mpeg, vob, asf, mxf, rm (and unknown) -> no key. Confirmed on real files below. |
| 4 | `ClassifyHDR`: hdr10 (BT.2020 + smpte2084), hlg (arib-std-b67), none; Dolby Vision out of scope and documented | Met | `hdr.go:45-59`; Dolby Vision note in `doc.go:13-16`. Real file `hevc_main10_hdr10_ac3.mkv` -> `hdr10`; SDR files -> none. |
| 5 | `BitDepth` falls back to pixel format when `bits_per_raw_sample` missing | Met | `bitdepth.go:30-36`. Real files: `hevc_main10_ac3.mkv` rawbits="" pix=yuv420p10le -> 10; `h264_aac.mkv` rawbits="8" -> 8; mpeg2/vp9/av1 rawbits="" pix=yuv420p -> 8. |
| 6 | `PrimaryVideo` skips attached pictures, prefers default; `DefaultAudio` prefers default else first; `AudioByIndex` -> `ErrNotFound` for missing/non-audio | Met | `description.go:67-112`; tests `description_test.go:84,134,172` (`:193` asserts `errors.Is(err, domain.ErrNotFound)`). |
| 7 | `Description.Validate()` rejects negative durations/sizes and duplicate stream indexes with `ErrInvalid` | Met | `description.go:27-63` (negative duration, negative bit rate, duplicate index, per-stream negatives); `file.go:48` rejects negative size; tests `description_test.go:45-78`, `file_test.go`. |
| 8 | Tests table-driven, no mocks; coverage >= 90 % | Met | 100.0 % of statements; no mock framework imports (imports list above). |
| 9 | Gates: lint + layer rules (archtest, depguard) + coverage gate | Met | golangci-lint `0 issues.` (depguard is part of `.golangci.yml`), archtest ok, go-test-coverage PASS. |

## Cross-check against real ffprobe

Method: scratch module outside the repo
(`C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\xcheck`,
`replace` to the repo) ran `ffprobe -v error -print_format json -show_format -show_streams` on all 22
files in `testdata/media/video/` (corpus already present, not regenerated), fed `format_name`
and extension to `IdentifyContainer`, compared the result with `expected.container` of
`testdata/media/manifest.json`, and ran `BitDepth`, `ParseFrameRate` (r_frame_rate and
avg_frame_rate) and `ClassifyHDR` on the video streams. Result: `mismatches: 0`.

| File | format_name | ext | Container | Direct-play key / MIME |
|---|---|---|---|---|
| av1_opus.webm, vp9_opus.webm | matroska,webm | .webm | webm | webm / video/webm |
| h264_aac.mkv, hevc_main10_ac3.mkv, hevc_main10_hdr10_ac3.mkv | matroska,webm | .mkv | mkv | mkv / video/x-matroska |
| h263_aac.3gp | mov,mp4,m4a,3gp,3g2,mj2 | .3gp | 3gp | 3gp / video/3gpp |
| h264_aac_faststart.mp4 | mov,mp4,... | .mp4 | mp4 | mp4 / video/mp4 |
| h264_aac.m4v | mov,mp4,... | .m4v | m4v | mp4 / video/mp4 |
| h264_pcm.mov | mov,mp4,... | .mov | mov | mov / video/quicktime |
| h264_ac3.m2ts, h264_ac3.mts | mpegts | .m2ts / .mts | m2ts | mpegts / video/mp2t |
| mpeg2_mp2.ts | mpegts | .ts | mpegts | mpegts / video/mp2t |
| theora_vorbis.ogv | ogg | .ogv | ogg | ogg / video/ogg |
| h264_aac.flv | flv | .flv | flv | none |
| mpeg2_ac3.vob | mpeg | .vob | vob | none |
| mpeg2_mp2.mpg | mpeg | .mpg | mpeg | none |
| mpeg4_*.avi (2 files) | avi | .avi | avi | none |
| wmv2_wmav2.wmv | asf | .wmv | asf | none |
| mpeg2_pcm.mxf | mxf | .mxf | mxf | none |
| rv20_ra144.rm / .rmvb | rm | .rm / .rmvb | rm | none |

Video stream values: all files 25/1 (parsed 25.000, no errors) for both frame-rate fields;
`hevc_main10_hdr10_ac3.mkv` -> bit depth 10, primaries bt2020, transfer smpte2084, HDR `hdr10`;
`hevc_main10_ac3.mkv` -> bit depth 10, HDR none; every other video 8 bit (ffprobe reports
`bits_per_raw_sample` only for h264/flv/mkv, the pixel-format fallback covers the rest); no
attached pictures in the video corpus. The corpus has no NTSC rate, so 24000/1001 rounding is
covered only by the unit tests (`framerate_test.go`).

## Implementer decisions evaluated

| Decision | Assessment |
|---|---|
| `ContainerUnknown = "unknown"` | Acceptable. Non-empty, so it round-trips through storage and the API; `ParseContainer` accepts it, and the zero value `""` is rejected by `Description.Validate`, which catches uninitialised descriptions. Consistent with the criterion "unknown input -> `ContainerUnknown`". |
| 3g2 -> 3gp | Acceptable. Documented in `isoMediaMember`; the ADR has no 3g2 key and both share the same MIME family. Tested (`container_test.go:78`). |
| ISO media without known extension -> mp4 | Acceptable. Documented; format name still decides the family (a file named `.mkv` that is really mp4 is identified by its bytes). |
| Extra `Stream.CodecTag` | Acceptable. The corpus manifest already carries `codecTag` (DIVX/XVID), so the field is useful for the technical chips. |
| Extra helpers (`ParseContainer`, `ParseStreamKind`, `ParseFrameRate`, `File.Validate/Probed`, probe error codes) | Acceptable. All covered at 100 %, documented, wrap `domain.ErrInvalid`. `ProbeError` codes (`probe_failed`, `probe_timed_out`) are a reasonable stable-code set; the doc comment says readers must tolerate new ones. |
| `AudioByIndex` -> `ErrNotFound` | Matches this task's criterion 6 exactly. M2-09 asks for `ErrInvalid`; the orchestrator decided that M2-09 wraps it, so this is consistent. Note for M2-09: `errors.Is(err, domain.ErrNotFound)` must be translated, not passed through, or the HTTP layer would answer 404 for a bad `audio` request parameter. |

Minor, non-blocking: the `File.Description` comment says "zero value when ProbeError is set", but
`validateFailedProbe` only enforces the absence of streams (container, duration and bit rate of a
failed-probe description are not checked). Harmless; tighten or reword when touched next.

## History

- Round 1 (2026-10-30): PASS.

REPORT
status: PASS
task: M2-02
files: docs/reports/M2-02-test.md
tests: go vet ./internal/domain/... clean; go test -race -count=1 ./internal/domain/media/ ok, 100.0 % (domain tree all 100 %); go-test-coverage PASS (70 % total, 90 % domain override); golangci-lint run + fmt --diff on the package clean; archtest ok; go list imports stdlib + internal/domain only; ffprobe cross-check of 22 corpus videos: 0 mismatches
findings:
- All 9 acceptance criteria met; implementer decisions acceptable.
- ADR 0023 table matches DirectPlayKey/MIMEType exactly (verified on real files).
- Note for M2-09: AudioByIndex returns ErrNotFound; it must wrap/translate to ErrInvalid so HTTP does not answer 404 for a bad audio parameter.
- Minor: File.Description comment ("zero value when ProbeError is set") is stronger than validateFailedProbe, which only checks streams.
- Out of scope, not judged: golangci-lint typecheck error in scripts/gen-testmedia tests (M2-06: undefined seekClipDuration/seekCaseSuffix/seekKeyframeInterval); go build error in internal/adapters/http/restapi/api.go:15 (unused import of internal/domain).
next: Send to mivo-reviewer; repo-wide gates (go test ./..., golangci-lint run, go-test-coverage on coverage.out) must be rerun once wave 1 compiles again.
