# M5-15 Search contract, use case, endpoint
Owner: mivo-backend
Depends on: M5-14
Files/areas: `api/openapi.yaml`, generated code, `internal/app/` (search use case), `internal/adapters/http/`, `cmd/mivo`

## Goal
`GET /api/v1/search?q=&type=&limit=&cursor=` serves the dropdown (top results across types) and
the results page (pivots by type) of SR-01/SR-02.

## Acceptance criteria
- [ ] `type` in top, movies, shows, episodes, music (artists/albums/tracks), photos,
      audiobooks; `top` returns <= 8 mixed hits with thumbnail image id, year, type, library.
- [ ] Queries shorter than 2 characters answer 400; paging by opaque cursor on typed pivots.
- [ ] Use case tests with fakes (type routing, limit clamp); handler tests incl. problem bodies.
- [ ] Spec validates; generated code diff clean.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Result visibility goes through the same `visibleLibraries` port as M5-13 (M7 restrictions).
