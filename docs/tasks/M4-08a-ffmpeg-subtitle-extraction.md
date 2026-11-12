# M4-08a FFmpeg: extract and convert text subtitles to WebVTT
Owner: mivo-media
Depends on: M4-03
Files/areas: `internal/adapters/ffmpeg/subtitle.go`, `subtitle_args.go` (+ tests, corpus test)

## Goal
Any text subtitle, embedded or external, becomes WebVTT with cue times relative to the file (ADR 0030 §3).

## Acceptance criteria
- [ ] `ExtractWebVTT(ctx, source, track, destination)` runs `ffmpeg -i <file> -map 0:s:<n> -c:s webvtt` (embedded) or reads an
      external file (via `os.Root`, bounded size) and writes UTF-8 WebVTT atomically (temp name + rename).
- [x] Source encodings other than UTF-8: a BOM (UTF-8/16/32) is left to FFmpeg's demuxer (an explicit `-sub_charenc` on a BOM file drops the cue text, verified by the tester); invalid UTF-8 is read as CP1252 via `-sub_charenc` (criterion amended by the orchestrator 2026-11-12);
      no charset guessing beyond BOM and UTF-8 validity (fallback CP1252).
- [ ] Image formats are refused with `domain.ErrInvalid`; a 2 h embedded track (built by the test, not a corpus file) extracts within 10 s (timeout 30 s).
- [ ] ASS tags are stripped by FFmpeg's conversion; the test asserts plain cue text for the styled corpus sample.
- [ ] Tests: arg builder table tests; corpus tests for SRT, ASS, mov_text, external SRT, UTF-16 sample; cancellation kills FFmpeg.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Normal priority is wrong here: run at low priority like transcodes (M4-07a helper).
- Output goes to a path the app layer chooses (M4-13c); the adapter does not know the cache layout.
