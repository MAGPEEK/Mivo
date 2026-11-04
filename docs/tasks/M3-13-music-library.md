# M3-13 Music library: artist → album → track from tags
Owner: mivo-backend (tag probing and covers: mivo-media)
Depends on: M3-11b (video slice merged), M3-18 green
Files/areas: to be fixed per sub-task (see Notes); expected: `internal/domain/naming` (music
folder fallback), `internal/domain/library` (kind `music`, item kinds), `internal/app/scan`
(music pipeline), `internal/adapters/ffmpeg` (audio tags, cover extraction),
`internal/adapters/sqlite` (migration `00004` for music fields if needed, reader/writer),
`api/openapi.yaml`, `internal/platform/config` (kind), `scripts/gen-testmedia` (music tree)

## Goal
A `music` library lists albums by artist with covers; tracks are ordered by disc and track
number and play through the existing playback path (audio direct play or transcode). Plan
milestone 3 "Musik-Tags", screen 5 data (artist and album pages arrive with M5). Second wave;
ADR 0027 §10.

## Acceptance criteria
- [ ] Config and domain accept `kind: music`; the contract adds `music` to `LibraryKind` and
      `artist|album|track` to `ItemKind`, plus `discNumber` and artwork kind `square`.
- [ ] The walker classifies audio files (`flac m4a alac mp3 aac opus ogg wav aiff ape wv dsf
      dff`) for music libraries; the input whitelist (ADR 0022 M2-24b) already covers the
      audio demuxers – any addition needs an ADR 0027 note.
- [ ] Placement from embedded tags (`album_artist` else `artist`, `album`, `disc`, `track`,
      `title`, `date`, `genre`, MusicBrainz IDs as provider IDs `musicbrainz_artist`,
      `musicbrainz_album`, `musicbrainz_track`); without tags the folder layout
      `Artist/Album/NN Title.ext` is used; "Various Artists" compilations stay one album.
- [ ] Album covers: `cover`, `folder`, `front` image files in the album folder, then the
      embedded picture; artist images `artist.jpg` in the artist folder.
- [ ] Corpus additions (tagged FLAC/MP3/M4A albums, a compilation, an untagged folder) and an
      integration test asserting the manifest; E2E: a music library lists an album with its
      cover and plays one track.
- [ ] Gates: all Go and web gates of the touched areas.

## Notes
- **Split (2026-11-04):** M3-13a domain, 13b contract, 13c ffmpeg tags/covers, 13d sqlite
  (migration 00005), 13e scan, 13f corpus, 13g wiring + tile, 13h web + E2E. See
  M3-00-overview "Second wave".
- Lyrics (LRC), ReplayGain and gapless playback are plan items of the music player (M5) and
  not part of this task; store ReplayGain tags only if the probe already exposes them.
- The probe today skips files without a video stream (ADR 0022); the music pipeline must not
  reuse that rule.
