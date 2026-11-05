# M3-13c FFmpeg: audio tag probe and embedded cover extraction
Owner: mivo-media
Depends on: M3-13a, M3-08
Files/areas: `internal/adapters/ffmpeg` (new files `audiotags.go`, `cover.go`)

## Goal
Read the tags and duration of an audio file and extract its embedded picture, with the bundled FFmpeg only.

## Acceptance criteria
- [x] `ReadAudioTags(ctx, path)` returns `media.AudioTags` (case-insensitive tag keys, `track`/`disc` as `3/12` forms, MusicBrainz keys, `date` -> year) plus duration; audio-only files are accepted (the video-stream rule of ADR 0022 is not applied).
- [x] `ExtractCover(ctx, path)` returns the attached picture (mjpeg/png stream with `attached_pic`) as bytes bounded by a size constant, or `ErrNoCover`.
- [x] Both honour the input whitelist and context cancellation.
- [x] Tests: integration against generated FLAC/MP3/M4A with and without cover (corpus from M3-13f, or generated in the test with FFmpeg); skip under `-short`.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Satisfies ports the scan declares in M3-13e (consumer-owned interfaces: `AudioTagReader`, `CoverExtractor`); keep names compatible or add a thin adapter in `cmd`. No new whitelist entries: audio demuxers are already allowed (M2-24b).

Implementation (M3-13c): `(*Prober).ReadAudioTags(ctx, path) (media.AudioTags, time.Duration, error)`
and `(*CoverExtractor).ExtractCover(ctx, path) ([]byte, error)`; both signatures use only domain
and standard types, so the M3-13e ports can match them without an adapter. `ErrNoCover` wraps
`domain.ErrNotFound`, so `app/scan` can test `errors.Is(err, domain.ErrNotFound)` without
importing the adapter. Ogg/Opus comments live on the audio stream and are merged under the
container tags. The extractor copies the picture (`-c copy`, no re-encode), prefers the stream
commented "Cover (front)", accepts mjpeg/png only and caps the output at 16 MiB.
