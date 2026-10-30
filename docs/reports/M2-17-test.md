# M2-17 test report – FFmpeg adapter: HLS remux and transcode arguments

Status: **PASS** (round 1)
Date: 2026-10-30
Environment: Windows 11, Go 1.27.3, FFmpeg/ffprobe 9.0.2 (gyan.dev, on PATH), golangci-lint 2.15.0.
Linux: race test binary built in `golang:1.27.3` (`--network none`, repo `:ro`), run in the local `mivo:dev`
image (FFmpeg 7.1.5-0+deb13u1, `--network none`, repo `:ro`). Tested together with M2-16 (same package); the
shared gate results are in `docs/reports/M2-16-test.md` and repeated here for the M2-17 files.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| vet | `go vet ./internal/adapters/ffmpeg/... ./internal/domain/...` | exit 0 (`go vet ./...` fails only in `internal/app/scan`, another task in flight) |
| tests, race | `go test -race -count=1 -v -coverprofile=<scratchpad> ./internal/adapters/ffmpeg/...` | `ok ... 26.226s coverage: 99.0%`; 271 PASS, 0 FAIL, **0 SKIP** |
| integration ran | same `-v` run | `TestHLSArgumentsProduceTheirPlanWithFFmpeg` PASS with 9 sub-runs: audio-flac-24bit-96k, video-mkv-h264-aac, video-avi-xvid-mp3, video-mkv-hevc10-ac3, video-m2ts-h264-ac3, video-mkv-h264-aac-long from 13s, ... as mpegts, ... as mpegts from 21s, video-ts-mpeg2-mp2-long from 13.3s |
| tests, `-short` | `go test -race -count=1 -short -v ...` | `ok`; `TestHLSArgumentsProduceTheirPlanWithFFmpeg` SKIP ("skipping FFmpeg integration test in -short mode"), table tests run |
| repeat | `go test -race -count=5 -run 'Capab\|HLSArg\|DetectCap'` | `ok 28.046s`, no flake |
| coverage | `go tool cover -func` | every function in `hlsargs.go` 100.0 %; package 99.0 %; `go-test-coverage` gate exit 0 (total >= 70 % satisfied); `internal/domain/...` all 100.0 % (>= 90 %) |
| lint | `golangci-lint run --allow-parallel-runners ./internal/adapters/ffmpeg/...` (windows and `GOOS=linux`) | `0 issues.` both |
| format | `golangci-lint fmt --diff ./internal/adapters/ffmpeg/...` (windows and `GOOS=linux`) | no diff |
| layer rules | `go test -count=1 ./internal/archtest/...`; `go list -deps ./internal/domain/...` filtered for forbidden imports | `ok`; no match |
| Linux, FFmpeg 7.1.5 | race test binary in `mivo:dev`, cwd = package dir, `-test.v -test.count=1` | exit 0; 272 PASS, 0 FAIL, 2 Windows-only SKIP (`TestHLSArgumentsRejectsWindowsDeviceOutputDirectories`, `TestProbeRejectsWindowsDevicePaths`); all 9 `TestHLSArgumentsProduceTheirPlanWithFFmpeg` sub-runs PASS (not skipped); no `DATA RACE` |
| stray files | `git status`, repo root | none from this run (overlays, fakes, outputs are in the scratchpad) |

## Independent run of the produced arguments (not the repo's tests)

An overlay test (a `_test.go` file mapped into the package through `go test -overlay`, nothing written into the
repo) called `Probe`, `playback.Decide` (Chromium profile, full capabilities, 1080p cap) and `HLSArguments` for three
corpus files and dumped the argument lists. I then ran `ffmpeg @args` myself and verified with `ffprobe`.
The same was done inside `mivo:dev` with FFmpeg 7.1.5 (Linux build of the same overlay, shell scripts with the
quoted arguments).

| Case | Plan | FFmpeg 9.0.2 (Windows) | FFmpeg 7.1.5 (mivo:dev) |
| --- | --- | --- | --- |
| remux `video-mkv-h264-aac` | remux, copy+copy | exit 0, stderr empty; files `index.m3u8 init.mp4 seg-00000.m4s`; h264 High 320x240 yuv420p + aac LC 2 ch; first video packet 0.08 s `K` | same; identical segment size (99513 bytes), duration 2.02 s |
| transcode `video-avi-xvid-mp3` | transcode video, copy audio | exit 0, empty stderr; h264 High yuv420p 320x240 (even) + mp3 copy; first video 0.08 s `K` | same; segment size identical (117093 bytes) |
| seek `video-mkv-h264-aac-long` at 13 s | remux, `-ss 12.000000` (`KeyframeAtOrBefore`(13 s) = 12 s) + `-copypriorss 0` | exit 0; 3 segments; first video packets 0.08 / 6.08 / 12.08 s all `K`; first audio 0.091 / 5.9998 / 12.016 s; duration 18.09 s | same; segment sizes identical (313731 / 318790 / 317002); duration 18.00 s |

Checked on every case on both versions:
- `-protocol_whitelist file` directly before `-i` (index 5 before `-i` at 7; with `-ss`: 7 before 9).
- Playlist has `#EXT-X-PLAYLIST-TYPE:EVENT`, `#EXT-X-INDEPENDENT-SEGMENTS`, `#EXT-X-MAP:URI="init.mp4"`,
  relative `seg-NNNNN.m4s` names and exactly one `#EXT-X-ENDLIST`. The directory holds only `index.m3u8`,
  `init.mp4` and `seg-*.m4s` (no temp files).
- Every segment (init + segment joined, probed with `ffprobe -show_entries packet=pts_time,flags`) starts with a
  keyframe packet (`K__`); output video dimensions 320x240 (even).
- Seek: the output begins at the keyframe the session reports. The first decoded frame of the output has the same
  md5 as the source frame at 12 s (`08e5ca11fcb5d96e9a6a9f3844dcadf9` on both FFmpeg versions; the source's keyframes
  are at 8/10/12/14 s), the output timeline starts at 0 (video 0.08 s = B-frame delay, no `-copyts`), and audio
  starts with the video.

## Mutation checks (overlay of `hlsargs.go`, repo untouched)

| Mutation | Tests that fail |
| --- | --- |
| remove the `-copypriorss 0` argument | `TestHLSArgumentsPerPlanShape/{seek_start_goes_before_the_input,sub-millisecond_seek_start_keeps_microseconds}` and the integration runs `TestHLSArgumentsProduceTheirPlanWithFFmpeg/{video-mkv-h264-aac-long_from_13s,video-mkv-h264-aac-long_as_mpegts_from_21s}` |
| copy AAC without `-bsf:a aac_adtstoasc` | 8 `TestHLSArgumentsPerPlanShape` rows (remux, HEVC copy, downscale, odd dimensions, mapping, seek ...) and the integration runs `TestHLSArgumentsProduceTheirPlanWithFFmpeg/{video-mkv-h264-aac-long_as_mpegts,..._as_mpegts_from_21s}` (the FFmpeg HLS muxer rejects ADTS) |

Both options are therefore guarded by argument tests and by real FFmpeg runs.

## Acceptance criteria

1. **`HLSArguments(job)`, `domain.ErrInvalid` for direct play or empty paths: met.**
   `TestHLSArgumentsRejectsJobsItCannotProduce` rows `direct_play_plan`, `empty_input_path`,
   `empty_output_directory`, `relative_*`, `negative_start`, `percent_sign_in_the_output_directory` assert
   `errors.Is(err, domain.ErrInvalid)` (`hlsargs_test.go:500`); unsupported encoders give `domain.ErrUnsupported`.
2. **Always-present arguments: met**, with three additions that go beyond the literal list and are documented
   in code and in the M2-24/M2-36 task notes: `-protocol_whitelist file` before `-i` (orchestrator note of this
   task, ADR 0022), `-bsf:a aac_adtstoasc` on every AAC copy, `-copypriorss 0` when `-ss` is set. `-ss <start>`
   is emitted only for start > 0 and before `-i`; input is `-i file:<input>`; `-map 0:<index>` per stream;
   `-sn -dn -map_metadata -1 -map_chapters -1`; the `-f hls ...` block (`-hls_time 6`, `event`, `fmp4`,
   `init.mp4`, `independent_segments+temp_file`, `-start_number 0`, `file:<dir>/seg-%05d.m4s`,
   `file:<dir>/index.m3u8`) is byte-for-byte as specified, verified in the dumped lists above. Output paths use
   `/` (FFmpeg's HLS muxer derives the init directory from the last `/`).
3. **Copy arguments: met.** `-c:v copy`, `-c:a copy`, `-tag:v hvc1` for HEVC (`HEVC_copy_is_tagged_hvc1`, and
   the integration run `video-mkv-hevc10-ac3` probes `hvc1`).
4. **Transcode arguments: met.** `libx264 -preset veryfast -crf 23 -profile:v high -pix_fmt yuv420p
   -force_key_frames expr:gte(t,n_forced*6) -sc_threshold 0`, `-vf scale=W:H` whenever `ScaleRequired` (0 dimension
   written as `-2`), audio `aac -ac N -b:a 192k`; values are named constants (`hlsargs.go:38-52`).
   `TestHLSArgumentsScaleTheSizeTheEngineDecided` (odd width/height, height cap, even size within the cap) PASS.
5. **Table tests, one row per plan shape: met.** `TestHLSArgumentsPerPlanShape` has 13 rows: remux, video-only
   transcode, audio-only transcode, both, downscale, odd-dimension rounding (720x405 -> 720x404), seek start,
   sub-millisecond start, no audio, no video, HEVC copy, unknown source width, stream-index mapping.
6. **Integration test: met** (more than asked: 9 runs). The three requested shapes (remux MKV H.264, transcode
   AVI, audio-only transcode M2TS) are `video-mkv-h264-aac`, `video-avi-xvid-mp3`, `video-m2ts-h264-ac3`; the
   playlist ends with `#EXT-X-ENDLIST`, `init.mp4` + segments joined probe as the planned codecs, size and
   channels. Runs on 9.0.2 and 7.1.5; skipped with `-short` or without FFmpeg/corpus.
7. **Gates: met** (table above).
8. **Notes of the task: met.** FFmpeg 7.1 verified in the container (above). `aac_adtstoasc`: the note
   ("needs none for the mp4-based muxer; verify") is answered by the implementation: the HLS muxer *does* need it
   for ADTS input (mutation B fails on the MPEG-TS runs); it is recorded in the code comment and the M2-24/M2-36
   tasks. `-ss` before `-i` without `-copyts`: output timeline starts at 0 (see seek case). Protocol whitelist
   asserted by `TestHLSArgumentsPassesTheProtocolWhitelistBeforeTheInput`; `ScaleRequired` drives the scale filter.

## Findings

None blocking.

- Observation (no action): `go vet ./...` and the repo-wide test run currently fail in packages outside this
  task (`internal/app/scan`, `internal/app/playback`, `internal/adapters/sqlite`, `cmd/mivo`); re-run the
  repo-wide gates when those tasks land.
- Observation (no action): `-bsf:a aac_adtstoasc` and `-copypriorss 0` are not in the task's literal argument
  list; consider adding them to the task/ADR 0024 text so the documented list matches the code (M2-36 already
  records them).

## History

- Round 1 (2026-10-30): PASS.
