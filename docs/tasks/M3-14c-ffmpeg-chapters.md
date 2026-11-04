# M3-14c FFmpeg: chapter probe
Owner: mivo-media
Depends on: M3-14a, M3-13c
Files/areas: `internal/adapters/ffmpeg` (new file `chapters.go`)

## Goal
Read the chapter list of an M4B/MP4/MKV/MP3 file.

## Acceptance criteria
- [ ] `ReadChapters(ctx, path)` returns `[]media.Chapter` from `-show_chapters` (time base converted to ms, empty title -> "Chapter n"); no chapters -> empty slice, no error.
- [ ] Integration test with a generated M4B with 3 chapters (ffmetadata) and a file without chapters; skip under `-short`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Also usable for video chapters in M4; keep it independent of audio specifics.
