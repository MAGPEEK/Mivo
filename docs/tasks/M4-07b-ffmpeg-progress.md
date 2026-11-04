# M4-07b FFmpeg: transcode progress (speed, fps, bit rate)
Owner: mivo-media
Depends on: none
Files/areas: `internal/adapters/ffmpeg/progress.go`, `producer.go` (+ tests); port addition in `internal/app/playback/ports.go` only as an optional `ProgressReporter` method on `Production`

## Goal
The server knows how fast a transcode runs (needed for the "too slow" hint, the sessions list and PL-41).

## Acceptance criteria
- [ ] FFmpeg runs with `-progress pipe:1 -nostats`; stdout key=value blocks are parsed (`out_time_us`, `speed`, `fps`,
      `bitrate`, `total_size`, `progress`) into `Progress{OutputTime, Speed, FPS, BitRate}`.
- [ ] `Production.Progress()` returns the latest snapshot without blocking; updates are coalesced to ≤ 1 per second.
- [ ] Malformed lines and `speed=N/A` are ignored; the reader drains stdout so FFmpeg never blocks on a full pipe.
- [ ] Tests: parser table tests on recorded output (software 1080p, remux); corpus test asserts Speed > 0 for a transcode.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Output bit rate for copied streams may be `N/A`; the sessions list then omits it.
- Do not log progress lines (volume).
