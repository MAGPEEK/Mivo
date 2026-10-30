# M2-26 REST API: libraries, scans and items
Owner: mivo-backend
Depends on: M2-01, M2-08, M2-18, M2-19
Files/areas: `internal/adapters/http/restapi/library.go` (+ new `library_*.go`, `items.go` and
tests)

## Goal
The library endpoints of the contract return real data: library list with availability and scan
status, scan trigger, paged items and item detail (ADR 0017, ADR 0022).

## Acceptance criteria
- [ ] `LibraryService` and `ScanService` interfaces in restapi (consumer-declared, small) are
      satisfied by `app/library.Service` and `app/scan.Service`; the M2-01 stubs are replaced.
- [ ] `GET /api/v1/libraries` and `GET /api/v1/libraries/{libraryId}` combine library data with
      the scan status; unknown ID → 404 problem.
- [ ] `POST /api/v1/libraries/{libraryId}/scan` → 202 with the status (also when a scan already
      runs); after server shutdown began → 503 `unavailable`.
- [ ] `GET …/items` maps `limit`/`cursor` (invalid cursor or limit → 400 `invalid`), returns
      `nextCursor` only when more items exist; `GET /api/v1/items/{itemId}` returns streams; file
      entries contain the file **name** only, never a path (test asserts no `/` or `\` in it).
- [ ] Durations in seconds as numbers with millisecond precision; timestamps RFC 3339 UTC.
- [ ] Handler tests with fake services (`httptest`): every status code of the contract, JSON shape
      against the generated types, logs contain truncated paths (M2-08).
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/http/...` + coverage gate.

## Notes
- Mapping functions domain → apigen live next to the handlers; keep each ≤ 50 lines.
- Do not edit `playback.go` (M2-28) or `api.go` (M2-32 makes services mandatory).
