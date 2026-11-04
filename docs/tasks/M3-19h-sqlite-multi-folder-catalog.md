# M3-19h SQLite: catalog writer and reader per folder
Owner: mivo-backend
Depends on: M3-19f, M3-19i (adds `RootID` to the scan records)
Files/areas: `internal/adapters/sqlite/` catalog writer v2 files (placement, sidecars,
metadata/artwork halves of M3-05b/M3-05d), reader v2 and artwork lookup (M3-05c), the file
lookup used by playback and images, their `queries/*.sql`, regenerated `sqlcgen/*`, tests

## Goal
Files, sidecars and local artwork are stored and found per library folder, so a library with
several folders scans, plays and shows posters correctly.

## Acceptance criteria
- [ ] The writer stores `root_id` from each record; change detection and removals are per
      `(root_id, relative_path)`; a record whose folder does not belong to the library is
      refused with `domain.ErrInvalid` (writer contract style of ADR 0022 Notes M2-40).
- [ ] Every read that turns a file, sidecar or local artwork into an absolute path (playback
      file resolution, image source lookup, sidecar reads) joins `library_roots` and returns the
      folder's path as the root, so callers keep opening files through `os.Root` of that folder
      unchanged.
- [ ] The primary-file and version ordering of ADR 0027 §5 stays deterministic across folders
      (`part_number, folder position, relative_path`).
- [ ] Tests against real SQLite: two folders with the same relative path (two versions of one
      movie, distinct rows), removal of one folder's file, lookups return the right folder
      path, refusal of a foreign `root_id`; all existing sqlite tests green.
- [ ] `sqlc` output committed; `sqlc diff` clean.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/sqlite/...`, coverage gate.

## Notes
- The migration (M3-19f) already wrote `root_id` for one-folder libraries; this task only makes
  the value come from the record and the lookups folder-aware.
- If a reader port in `app/library`, `app/playback` or `app/images` returns a library root
  today, keep its field name and fill it with the folder path – no port change is expected;
  report one if needed.
