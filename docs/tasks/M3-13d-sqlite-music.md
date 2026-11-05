# M3-13d SQLite: migration 00006 music fields, writer and reader
Owner: mivo-backend
Depends on: M3-13a, M3-19h (multi-folder catalog)
Files/areas: `internal/adapters/sqlite/migrations/00006_*.sql`, `queries/music*.sql`, `sqlcgen/` (generated), `library_catalog_music.go`, tests

## Goal
Persist artists, albums and tracks and read them back as a hierarchy ordered by disc and track.

## Acceptance criteria
- [ ] Migration `00006` adds `items.disc_number` and the artwork kind `square` value if the schema constrains it; no table rebuild.
- [ ] Writer upserts artist/album/track items with stable IDs from the match keys and MusicBrainz IDs as provider IDs.
- [ ] Reader lists albums of an artist, tracks of an album ordered by (disc, track, file name), and albums of a library with paging.
- [ ] Tests: real SQLite in a temp dir, migration up from `00004`, ordering and paging cases.
- [ ] Gates: lint + layer rules + coverage thresholds; `sqlc generate` leaves the diff clean.

## Notes
Number `00006` is reserved for music, `00007` for audiobooks (M3-14d), `00008` for photos (M3-15e) (00005 is M6-03 online metadata); if a task merges out of order it renumbers on rebase. Docker for sqlc needs the usual approval.
