# M3-14d SQLite: migration 00008 `media_chapters`, writer and reader
Owner: mivo-backend
Depends on: M3-14a, M3-13d
Files/areas: `internal/adapters/sqlite/migrations/00008_*.sql` (00007 is M6-11, merged first), `queries/chapters.sql`, `sqlcgen/`, `library_catalog_chapters.go`

## Goal
Store chapters per media file and read them per item in playback order.

## Acceptance criteria
- [ ] `media_chapters(media_file_id, ordinal, title, start_ms, end_ms)` with FK cascade and unique (file, ordinal); additive migration.
- [ ] Writer replaces a file's chapters atomically; reader returns a book's chapters across its files with cumulative book offsets.
- [ ] Tests: real SQLite, migration up from `00006`, replace and cascade cases.
- [ ] Gates: lint + layer rules + coverage thresholds; sqlc diff clean.

## Notes
Multi-file books: reader adds the preceding files' durations so the API exposes book-relative `startMs`/`endMs` plus `fileId`.
