# M4-06b SQLite: external subtitle table and reader
Owner: mivo-backend
Depends on: M4-05b
Files/areas: `internal/adapters/sqlite/migrations/0000N_external_subtitles.sql`, `queries/subtitles.sql`, `sqlcgen`, new adapter file

## Goal
External subtitles persist with their media file and are returned with the file's tracks (ADR 0030 §3).

## Acceptance criteria
- [ ] Migration (next free number; check music/audiobook/photo migrations 00005..00007 first) creates `external_subtitles`
      (`media_file_id` FK cascade, `track_id`, `relative_path`, `format`, `language`, `forced`, `sdh`, `is_default`).
- [ ] `ReplaceExternalSubtitles` is transactional; the catalog reader returns the rows with the file.
- [ ] Deleting a file or library cascades.
- [ ] Tests: integration against a temp-dir SQLite (replace, read, cascade, migration up on an existing 00004+ database).
- [ ] `sqlc generate` (Docker) leaves `git diff --exit-code` clean.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Migration number must follow whatever music/photo chains merged; coordinate through the overview "shared files".
