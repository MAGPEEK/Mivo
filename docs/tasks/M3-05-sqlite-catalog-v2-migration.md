# M3-05 SQLite: migration 00003 "catalog v2"
Owner: mivo-backend
Depends on: M3-01
Files/areas: `internal/adapters/sqlite/migrations/00003_catalog_v2.sql` (new),
`internal/adapters/sqlite/sqlcgen/**` (regenerated), `internal/adapters/sqlite/
media_catalog_schema_test.go` (or a new `catalog_v2_schema_test.go`), minimal compatibility
edits in `catalog_writer.go`, `queries/media.sql` and their tests, `docs/tasks/BACKLOG.md`
(only the `sqlite` nits row of M2-10)

## Goal
The database holds the library model v2 of ADR 0027 §5: hierarchy columns and match keys,
metadata, tags, people, provider IDs, field locks, artwork and sidecar states. The server keeps
working exactly as in milestone 2 on the new schema, so the writer and reader tasks of later
waves start from a green build.

## Acceptance criteria
- [ ] Goose migration `00003_catalog_v2.sql` drops `media_streams`, `media_files`, `items`,
      `libraries` and recreates them with the v2 columns, then creates `tags`, `item_tags`,
      `people`, `item_people`, `item_provider_ids`, `item_field_locks`, `artwork`,
      `library_sidecars` (ADR 0027 §5, OD-M3-1 A). `Down` restores the `00002` shape (empty).
- [ ] Closed sets have `CHECK`s: library kinds (five), item kinds (all of ADR 0027 §2), extra
      types, artwork sources, tag kinds, sidecar kinds (`nfo|image`), media file layouts.
      Growing sets (person roles, providers, artwork kinds, lockable field names) are only
      checked for non-empty text (ADR 0027 §5). Dates are `TEXT` checked with
      `GLOB '[0-9][0-9][0-9][0-9]-[0-1][0-9]-[0-3][0-9]'`.
- [ ] `items`: unique index on `(library_id, ifnull(parent_id, ''), kind, match_key)`; the
      keyset index for top-level listing `(library_id, sort_title, id) WHERE parent_id IS NULL
      AND kind <> 'extra'`; children index `(parent_id, number, sort_title, id)`.
- [ ] `artwork`: exactly one location group per source (`CHECK`), partial unique index
      `(item_id, kind) WHERE selected = 1`, `ON DELETE CASCADE` from items, media files
      (embedded) and libraries.
- [ ] Every foreign key column leads an index (ADR 0022 Notes M2-10); every new table is
      `STRICT`; BACKLOG `sqlite` nits row done where it concerns the rebuilt tables
      (`CHECK (sort_title <> '')`, `frame_rate` `NOT GLOB '*[^0-9/]*'`, HDR wording in the
      migration comment); delete the row (the `COLLATE NOCASE` and sync-query nits move to
      M3-05c if not done).
- [ ] The M2 writer keeps working on the new schema with the smallest change: it sets
      `match_key` to the file's relative path and leaves new columns at their defaults. All
      existing sqlite, app and cmd tests stay green (`go test ./...`).
- [ ] Schema tests: the migration applies on an empty database and on a `00002` database with
      rows (they are dropped), `goose down` to `00002` works, each `CHECK`/unique rule rejects a
      bad row (table test), and the artwork partial index rejects a second selected row.
- [ ] `docker run … sqlc/sqlc:1.31.1 generate …` output is committed; `sqlc diff` clean.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/... ./cmd/...`, coverage gate.

## Notes
- The migration is the one place that decides column names for all later sqlite tasks; take
  them from ADR 0027 §5 and report every deviation.
- New `NOT NULL` columns get defaults so M2 code compiles unchanged; sqlc model structs grow,
  which is the only reason the M2 code is touched.
- Do not write new queries for the v2 writer or reader here (M3-05b, M3-05c, M3-05d own their
  query files). `models.go` changes only in this task during milestone 3's first wave;
  second-wave migrations (music, photos) regenerate it again, one at a time.
- The release notes of the next version must mention the one-time re-probe (M3-11b collects
  release-note lines in its REPORT).
