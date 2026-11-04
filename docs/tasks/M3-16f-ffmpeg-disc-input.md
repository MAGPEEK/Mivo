# M3-16f FFmpeg: probe and producer input for disc folders
Owner: mivo-media
Depends on: M3-16a, M3-16c
Files/areas: `internal/adapters/ffmpeg` (new file `disc_input.go`), whitelist constants, ADR 0029 whitelist note

## Goal
Probe and stream a disc folder's main title with stock FFmpeg demuxers.

## Acceptance criteria
- [ ] `DiscTitle` -> FFmpeg input: ordered clips joined by the `concat` demuxer with a generated list file in a private temp dir (`-safe 0` only for validated absolute paths inside the library root), or the `concat:` protocol for VOB/m2ts byte-concatenation; `mpeg`/`mpegts` demuxers do the rest.
- [ ] Whitelists extended minimally and explicitly (`concat` format, `file` protocol for the list); any other protocol stays blocked; recorded in ADR 0029 (supersedes the relevant M2-24b note by addition).
- [ ] Probe returns duration, video/audio/subtitle streams of the main title (DVD subtitles bitmap -> flagged non-textual); missing demuxers -> `disc_unsupported`.
- [ ] Producer (remux/transcode pipeline) accepts the disc input; seeking works via `-ss` before input on the concat list.
- [ ] Tests: integration with the generated disc corpus (M3-16i) or small generated VOB/m2ts; skip under `-short`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
List-file lifecycle tied to the session (deleted on stop). No libbluray/libdvdnav (ADR 0029).
