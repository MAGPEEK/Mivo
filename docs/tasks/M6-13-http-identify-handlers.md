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
