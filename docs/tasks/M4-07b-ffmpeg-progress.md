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

## Implementation notes
- `Progress` and the optional `ProgressReporter` interface live in `internal/app/playback/ports.go`; `Production`
  itself is unchanged, callers type-assert. The FFmpeg production implements it.
- FFmpeg 9.0.2 reports `bitrate=N/A` and `total_size=N/A` for every HLS run (transcode and remux), so `BitRate`
  stays 0 in practice; the sessions list needs another source for the output bit rate (the plan's target bit rate
  or the segment sizes). `total_size` is recognised but not kept: FFmpeg derives `bitrate` from it.
- `out_time_us` and `speed` are `N/A` in the blocks while FFmpeg flushes; the last known values are kept.
- The last block (`progress=end`) is always published, so a run shorter than a second still has a snapshot.
