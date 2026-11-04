# M3-13c FFmpeg: audio tag probe and embedded cover extraction
Owner: mivo-media
Depends on: M3-13a, M3-08
Files/areas: `internal/adapters/ffmpeg` (new files `audiotags.go`, `cover.go`)

## Goal
Read the tags and duration of an audio file and extract its embedded picture, with the bundled FFmpeg only.

## Acceptance criteria
- [ ] `ReadAudioTags(ctx, path)` returns `media.AudioTags` (case-insensitive tag keys, `track`/`disc` as `3/12` forms, MusicBrainz keys, `date` -> year) plus duration; audio-only files are accepted (the video-stream rule of ADR 0022 is not applied).
- [ ] `ExtractCover(ctx, path)` returns the attached picture (mjpeg/png stream with `attached_pic`) as bytes bounded by a size constant, or `ErrNoCover`.
- [ ] Both honour the input whitelist and context cancellation.
- [ ] Tests: integration against generated FLAC/MP3/M4A with and without cover (corpus from M3-13f, or generated in the test with FFmpeg); skip under `-short`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Satisfies ports the scan declares in M3-13e (consumer-owned interfaces: `AudioTagReader`, `CoverExtractor`); keep names compatible or add a thin adapter in `cmd`. No new whitelist entries: audio demuxers are already allowed (M2-24b).
