# M4-09b FFmpeg: hardware encode arguments and software fallback
Owner: mivo-media
Depends on: M4-09a, M4-07a
Files/areas: `internal/adapters/ffmpeg/hwargs.go`, `hlsargs.go` (+ tests)

## Goal
A plan whose video encoder is hardware produces the matching FFmpeg arguments per API, and a failing hardware start falls back to
software without the viewer noticing more than a delay (ADR 0030 §5).

## Acceptance criteria
- [ ] With a bit-rate cap (Job.MaxBitrate) each hardware builder applies the cap through the shared `bitrateArguments`/`videoBudget` of M4-11 (nvenc `-b:v`/`-maxrate`/`-bufsize`, qsv, vaapi, amf per API) with golden arguments per API; an uncapped job is unchanged (moved from M4-11 by the orchestrator 2026-11-12).
- [ ] Per API an argument builder: VAAPI (`-vaapi_device`, `format=nv12,hwupload`, `h264_vaapi -qp`), QSV (`-init_hw_device qsv`,
      `h264_qsv`), NVENC (`h264_nvenc -preset p4 -rc vbr -cq`), AMF (`h264_amf -quality speed`); all keep `-g`/`-keyint` forced-keyframe
      behaviour of M2 (segment boundaries), 8-bit 4:2:0 output and the same scaling target.
- [ ] Hardware decode is enabled only for combinations the test run verified (the detection result carries `Decode bool`); otherwise
      software decode + hardware encode.
- [ ] `Start` retries once in software when the hardware process fails before the first segment; the retry is logged at info; the producer
      reports `Fallback=true` so the service can mark the encoder degraded (M4-13d).
- [ ] Software path output is byte-for-byte unchanged (existing golden tests).
- [ ] Tests: arg builder golden tests per API (with and without scaling and burn-in); fake-tool test for the fallback; no real GPU needed.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Real-hardware verification happens on the owner's machines or contributors' (not CI); record results in `docs/` (M4-18).
- Keep HEVC/AV1 output out of scope.
