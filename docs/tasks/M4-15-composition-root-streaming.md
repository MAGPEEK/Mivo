# M4-15 Composition root and integration test: streaming extension
Owner: mivo-backend
Depends on: M4-05a, M4-06a, M4-06b, M4-07a, M4-08b, M4-09b, M4-10, M4-11, M4-13a – M4-13e, M4-14a, M4-14b
Files/areas: `cmd/mivo/**`, integration test under `cmd/mivo` or `internal/adapters/corpustest`

## Goal
Everything of M4's P0 slice runs in the real binary: checkpoint of the milestone (walking skeleton for subtitles, tracks, throttling, hardware status).

## Acceptance criteria
- [x] Wiring: config → producer options (preset, resolved threads, priority), transcoder service (startup detection in background), subtitle
      and trickplay services, throttle controller, sessions list; no logic in `main`.
- [x] Integration test over `testdata/media` and `testdata/library/subs`: (a) SRT session = direct play + working `.vtt`; (b) PGS sample → transcode with
      `subtitle_burn_in`; (c) second audio track via `audioStreamIndex`; (d) 4 Mbit/s cap produces transcode; (e) `GET /system/transcoder` returns an
      empty or consistent device list; (f) throttle: on Linux/Docker a transcode pauses when the client stops requesting segments.
- [x] Shutdown budget test still passes (paused FFmpeg is killed within the 5 s bound).
- [x] `docker build` smoke (smoke script extended; image run left to the release gate) and `scripts/playback-smoke` extended with one subtitle and one 2-Mbit/s case (profile `firefox-desktop`).
- [x] Release notes snippet collected: preset/threads/priority defaults, `max_sessions` 1 on ≤ 4 CPUs.
- [ ] Gates: lint + layer rules + `go test -race -tags webui ./cmd/...` + coverage thresholds.

## Notes
- Checkpoint for DS925+ behaviour: see M4-18.

Note (orchestrator, M4-13b): map `config.Transcode.ThrottleAheadSeconds` into `playback.Options.ThrottleAhead` in the same change as M4-14a's segment hook (without the hook the controller measures against segment 0 and stalls playback past about segment 11); until then ThrottleAhead stays 0 (off).

Note (orchestrator, M4-07a): in `cmd/mivo/wire_playback.go` pass `ProducerOptions.Preset`, `Threads` (0 resolved to `max(1, NumCPU/2)`, clamped to a sane upper bound) and `LowPriority` (`Transcode.Priority == "low"`) from `config.Transcode`, with a wiring test; also set `LowPriority` for the subtitle extractor (M4-08a) and the trickplay generator (M4-12), and pass `scan.SubtitlePorts{Lister, Catalog}` (M4-06a/b).

Note (orchestrator, M4-10): call `ffmpeg.HasSoxResampler(ctx, ffmpegPath)` once at startup (bounded ctx) and pass the result to the producer so `HLSJob.SoxResampler` is set (add `ProducerOptions.SoxResampler` if it does not exist yet, producer.go builds the job); add a wiring test; FLAC capability (`FLACEncoder`) comes from detection already.

## Implementation notes (M4-15)

- Wiring lives in `cmd/mivo/wire_transcode.go` (producer options, transcoder service, hardware start guard),
  `wire_subtitles.go`, `wire_trickplay.go` and `wire_streaming.go` (services next to playback, started after the
  playback service, stopped in parallel with it inside the 5 s playback close bound).
- The playback service and the producer read the hardware encoders per session start (`playback.CapabilitySource`,
  `ffmpeg.HardwareEncoderSource`), because the detection finishes in the background after the listener started.
- `BurnInText` is reported `false` by `withSafeBurnIn`; the decision engine now refuses a text burn-in without it
  (`transcode_unavailable`, or the warning `subtitle_burn_in_unavailable` with `allowDegradeSubtitles`). Enabling text
  burn-in needs the hardened extraction of the embedded track into a session file and the seek offset fix first.
- Open: there is no runtime writer of `transcode.hwaccel` yet (configuration only), so `SetSetting` off -> auto has no
  caller; when a settings endpoint arrives it must call `Test` after the change.

## Release notes snippet (M4-15)

Transcoding defaults: x264 preset `veryfast`, encoder threads `0` = automatic (half of the CPUs, at least 1, at most
16), FFmpeg runs at low OS priority (`transcode.priority = low`), hardware acceleration `auto` (tested once in the
background after start, never blocks the listener) and a throttle that pauses FFmpeg 60 s ahead of the viewer (Linux
only). `transcode.max_sessions` defaults to 1 on machines with at most 4 CPUs and to 2 above. Text subtitles play as
WebVTT sidecars; only picture subtitles (PGS, VobSub) are burned in, a text subtitle a client cannot render answers
`transcode_unavailable` until the hardened text burn-in lands.
