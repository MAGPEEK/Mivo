# M3-06b REST: items v2 and item children
Owner: mivo-backend
Depends on: M3-02, M3-06
Files/areas: `internal/adapters/http/restapi/{items.go,items_response.go,library.go,
library_response.go}` and their tests, `library_fakes_test.go`, `docs/tasks/BACKLOG.md` (only the
`restapi library` row)

## Goal
The REST adapter maps the v2 read model to the contract of M3-02: items carry their kind,
hierarchy, metadata and image references, and `GET /api/v1/items/{itemId}/children` pages
through seasons and episodes. UX: DT-20 … DT-22, DT-24, LB-40, DT-06, DT-08, DT-09.

## Acceptance criteria
- [ ] `listItemChildren` is implemented (the 501 stub of M3-02 is gone): `limit`, `cursor`,
      `order` bound and validated (bad values → 400 `invalid`), unknown item → 404.
- [ ] Every new field of `ItemSummary`, `ItemDetail` and `MediaFile` is mapped; absent values
      are omitted, never sent as empty strings or zero numbers (season 0 is sent as `0`);
      `releaseDate` uses `format: date`; artwork kinds map one to one; `ImageRef.tag` is the
      stored tag.
- [ ] No absolute path and no library root appears in any response (test scans the JSON of a
      fixture item whose fake data contains a root).
- [ ] BACKLOG `restapi library` row done and deleted (reject an empty `?cursor=`; spec note
      that `\` counts as a directory separator in `fileName`).
- [ ] Table tests per mapper (movie, show, season with fallback poster, episode with
      siblings, extra) and handler tests for children paging and errors against fakes of
      M3-06's ports.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/http/...`, coverage gate.

## Notes
- Problem mapping stays in `problem.go` (unchanged); only use its existing helpers.
- Keep the response mappers small and table-driven; the detail mapper will grow in M6
  (ratings, portraits), so give each block its own function.
