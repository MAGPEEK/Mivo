# M3-13a Domain: music kind, audio files, tag-based placement
Owner: mivo-backend
Depends on: M3-04, M3-04b, M3-19b (domain only; no wave-2 code needed)
Files/areas: `internal/domain/library` (new files), `internal/domain/media` (`AudioTags`), `internal/domain/naming` (new file `music.go`)

## Goal
Pure rules that turn audio tags or a folder layout into artist, album and track placement, so
every later music task works against fixed types.

## Acceptance criteria
- [ ] `library.KindMusic`; item kinds `artist|album|track`; audio extension set (`flac m4a alac mp3 aac opus ogg wav aiff ape wv dsf dff`) as a file class valid only for music and audiobooks.
- [ ] `media.AudioTags` value (album artist, artist, album, disc, track, title, year, genre, MusicBrainz artist/album/track IDs) with a `Placement` function: album artist else artist; "Various Artists" and `compilation=1` stay one album; missing disc = 1.
- [ ] Folder fallback `Artist/Album/NN Title.ext` and `Artist/Album/CD n/NN Title.ext`; tags win over folders field by field.
- [ ] Sort title and track ordering by (disc, track, file name).
- [ ] Tests: table-driven, domain coverage >= 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No I/O; match keys follow ADR 0027 §3 (artist key, album key = artist key + album + disc-less year). Ports are not defined here (the scan declares them in M3-13e).
