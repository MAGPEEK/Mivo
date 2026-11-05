# M6-13 HTTP handlers: identify, refresh, provider settings, status
Owner: mivo-backend
Depends on: M6-01, M6-12
Files/areas: `internal/adapters/http/**` (handlers only, not apigen)

## Goal
Serve the M6-01 contract.

## Acceptance criteria
- [ ] All endpoints of M6-01 implemented; domain errors mapped only here (`ErrNotFound` 404,
      `ErrConflict` 409, `ErrUnavailable` 503 + `Retry-After`, `ErrInvalid` 400).
- [ ] Management endpoints use the ADR 0028 guard (non-local peer or foreign origin → 403).
- [ ] Settings PUT stores the TMDB key write-only; GET never returns it (test greps the
      response body); switching online off is effective for the next job.
- [ ] Candidate poster URLs are Mivo URLs (proxied through a short-lived, cached server fetch
      limited to the TMDB image host), never provider hosts in JSON (test).
- [ ] Items list supports `matchState=unmatched`; DTOs expose `matchState`, `metadataSources`.
- [ ] Tests: `httptest` against handlers with fake use cases; guard cases; `go test -race
      ./internal/adapters/http/...` and with `-tags webui`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The poster proxy reuses the outbound client; do not forward client headers or cookies.

Implementation (M6-13):
- The metadata services are optional `restapi.Options.Metadata` fields; an unwired operation
  answers 501 `not_implemented` as the stubs did, so M6-14 can wire them step by step.
- Candidate posters: API 1.11.0 adds `GET /api/v1/metadata/posters/{posterId}` (tag images,
  plain handler in `adapters/http/posterproxy`). The search registers each TMDB poster and gets
  an opaque 32-hex ID that lives one hour; the bytes come from `artworkfetch.Fetcher.Preview`
  (TMDB image host only, validated, not stored) and stay in memory within a 32 MiB budget.
- The runtime settings live in the new `app/metadatasettings.Service`, which is the
  `enrich.SettingsSource` and the TMDB key provider, so a change counts from the next job.
- `matchState`/`metadataSources` come from `EnrichmentStore.ItemMatchStatuses`; the filter
  `matchState` is a `library.ListingFilter` field with an SQL condition (no row = unmatched).
- identifyItem never contacts the provider, so its 503 was removed from the contract.
- Wiring and remaining notes: BACKLOG rows "m6-13 wiring" and "m6-13 notes".
