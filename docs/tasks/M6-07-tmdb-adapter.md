# M6-07 TMDB adapter: search and details
Owner: mivo-media
Depends on: M6-02, M6-04, M6-06
Files/areas: `internal/adapters/metadata/tmdb/**`, `testdata/providers/tmdb/**`, `scripts/record-provider-fixtures/**` (dev-only)

## Goal
Implement `MovieLookup` and `ShowLookup` against the TMDB v3 API, mapping responses to domain
records (title, original title, overview, tagline, genres, studios, countries, collection,
release date, content rating per region, cast/crew, provider IDs incl. IMDb/TVDB, image refs).

## Acceptance criteria
- [ ] Search by title+year and by external ID (`/find` for IMDb/TVDB), details with
      `append_to_response=credits,external_ids,release_dates|content_ratings,images`;
      season and episode details with in-language fallback to English for empty fields.
- [ ] Key sent as bearer/`api_key` only through the outbound client, redacted from errors;
      the bundled-or-own key is injected via a small `KeyProvider` interface.
- [ ] Image refs carry TMDB file paths; `image.tmdb.org` URLs are built by the adapter with a
      fixed size (`w780` poster, `w1280` backdrop, `original` limited by M6-08).
- [ ] HTTP 401 → `ErrUnavailable` with "invalid key" flag, 404 → `ErrNotFound`, 429 →
      `ErrRateLimited` (after client retries).
- [ ] Requests contain only query, year, language and IDs (assert on the recorded request in
      tests).
- [ ] Tests: `httptest` server serving fixtures recorded once (movie with and without
      collection, show, season, episode, non-English, empty result, 401, 404, 429);
      no test touches the network; `go test -race ./internal/adapters/metadata/tmdb/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Fixtures are committed, trimmed to needed fields, and contain no key. The recorder script is
never run in CI. Base URL is an option so tests and E2E (M6-17) can point to a fake.
