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

## Result (implementation notes)
- Contract 1.17.0: `GET /api/v1/search` (`operationId: search`), `SearchType`, `SearchResults`
  (`items` of `ItemSummary`, `nextCursor`). A hit is an `ItemSummary`, so it already carries
  kind, year, `libraryId` and the poster, thumb or square image reference.
- `type` defaults to `top` (at most 8 hits, default 8, never paged; a cursor answers 400). The
  pivots page with 24 hits by default and at most 50; `limit` outside 1–50 answers 400.
- `q` is trimmed and counted in characters: fewer than 2 or more than 200 answers 400
  `invalid` with field `q`. A missing `q` is the generic 400 `bad_request` of a missing
  required parameter.
- Use case `internal/app/search.Service.Search(ctx, Request)`; `Request.Restriction` is the
  viewer's `access.Restriction` and goes to the item summaries. The handler passes
  `access.Unrestricted()` until M7-14.
- Paging: the index port takes a limit but no offset, so a page asks the index for
  offset + limit + 1 hits (at most `MaxLimit` = 100) and cuts its window out of them. A pivot
  therefore ends after its 100 best hits; the contract says so. The cursor is base64url of the
  pivot and the offset and is refused for another pivot.
- At most 500 visible libraries per search (one bound SQL parameter each); kinds come from a
  fixed table per type, never from the client (M5-14 review minor).
- Without a search service the endpoint answers 501 `not_implemented`, like the rows.
- `cmd/mivo` wires `sqlite.SearchIndex`, `sqlite.CatalogReader` and the rows' `AllLibraries`.
