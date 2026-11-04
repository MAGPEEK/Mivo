# M3-13d SQLite: migration 00005 music fields, writer and reader
Owner: mivo-backend
Depends on: M3-13a, M3-19h (multi-folder catalog)
Files/areas: `internal/adapters/sqlite/migrations/00005_*.sql`, `queries/music*.sql`, `sqlcgen/` (generated), `library_catalog_music.go`, tests

## Goal
Persist artists, albums and tracks and read them back as a hierarchy ordered by disc and track.

## Acceptance criteria
- [ ] Migration `00005` adds `items.disc_number` and the artwork kind `square` value if the schema constrains it; no table rebuild.
- [ ] Writer upserts artist/album/track items with stable IDs from the match keys and MusicBrainz IDs as provider IDs.
- [ ] Reader lists albums of an artist, tracks of an album ordered by (disc, track, file name), and albums of a library with paging.
- [ ] Tests: real SQLite in a temp dir, migration up from `00004`, ordering and paging cases.
- [ ] Gates: lint + layer rules + coverage thresholds; `sqlc generate` leaves the diff clean.

## Notes
Number `00005` is reserved for music, `00006` for audiobooks (M3-14d), `00007` for photos (M3-15e); if a task merges out of order it renumbers on rebase. Docker for sqlc needs the usual approval.
