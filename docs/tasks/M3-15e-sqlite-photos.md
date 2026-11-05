# M3-15e SQLite: migration 00008 `photo_info`, writer and reader
Owner: mivo-backend
Depends on: M3-15a, M3-14d (migration order), M3-19h
Files/areas: `internal/adapters/sqlite/migrations/00008_*.sql`, `queries/photos.sql`, `sqlcgen/`, `library_catalog_photos.go`

## Goal
Persist photo albums, photos and EXIF values; list albums and photos by capture time.

## Acceptance criteria
- [ ] `photo_info(item_id PK, taken_at, make, model, lens, exposure, f_number, iso, focal_mm, orientation, lat, lon, width, height, thumbnail_state)`; additive, index on (parent, taken_at).
- [ ] Writer upserts album/photo items and info; alternate RAW file stored as a second media file of the photo.
- [ ] Reader: children of an album (sub-albums first, then photos by capture time, file name), paging by cursor, detail with info.
- [ ] Tests: real SQLite, migration up from `00007`, ordering, paging, 10 000-row paging smoke under 1 s.
- [ ] Gates: lint + layer rules + coverage thresholds; sqlc diff clean.

## Notes
Index only what the album view needs; the timeline query is M5.
