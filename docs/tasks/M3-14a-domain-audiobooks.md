# M3-14a Domain: audiobook kind, chapters, author and narrator roles
Owner: mivo-backend
Depends on: M3-13a
Files/areas: `internal/domain/library` (new files), `internal/domain/media` (`Chapter`), `internal/domain/naming` (new file `audiobook.go`)

## Goal
Pure rules for grouping audio files into books and for chapter values.

## Acceptance criteria
- [ ] `library.KindAudiobooks`, item kind `audiobook`, `PersonRole` gains `author` and `narrator`.
- [ ] `media.Chapter` (title, start, end, optional file reference) with validation (ordered, non-overlapping, end > start) and a function that builds chapters from ordered part files (one chapter per part, cumulative offsets).
- [ ] Grouping: one book per folder of audio files; a single M4B/M4A/MP3 in a shared folder is its own book; part order by (disc, track), else natural file-name order; title from `album` tag, else folder or file name; `composer` is narrator only when the tag says so.
- [ ] Tests: table-driven, domain coverage >= 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
`Chapter` is designed for video chapters too (M4, PL-30); record that in ADR 0027 Notes.
