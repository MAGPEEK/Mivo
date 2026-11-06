# M7-14 App: viewer-scoped catalog
Owner: mivo-backend
Depends on: M7-13, M7-15
Files/areas: `internal/app/access/` (scoped library source), `internal/app/library/`, `internal/app/rows/`, `internal/app/search/`, `internal/app/playback/` (start check), their tests

## Goal
Every catalog use case reads the viewer from the context and passes its restriction, so
enforcement happens on the server for every list, row, search, detail and stream start (US-12).

## Acceptance criteria
- [ ] `app/access.ScopedLibraries` replaces `rows.AllLibraries`: visible libraries are the
      viewer's effective access set; an inaccessible library id wraps `domain.ErrNotFound`.
- [ ] `app/library`, `app/rows`, `app/search` build the restriction from `access.ViewerFrom(ctx)`
      and remove every `TODO(M7-14)`; no viewer → `domain.ErrUnauthenticated` (fail closed).
- [ ] Starting a playback session for a hidden item or a file of a hidden item answers
      `domain.ErrNotFound`; a session records the auth session id that started it, and get,
      stop, file and HLS requests of another auth session (non-admin) answer `ErrNotFound`.
- [ ] Background jobs (scan, enrichment, scan trigger) do not depend on a viewer (test with a
      bare context).
- [ ] Tests: use-case tests with in-memory fakes (`go test ./internal/app/...`): limited viewer,
      admin viewer, missing viewer, library outside access, session ownership.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The playback ownership check refines ADR 0024: add a short Notes (M7-14) section there. Images
stay unscoped (ADR 0032 §7). Keep each service change small; if this exceeds ~400 lines, split
playback into M7-14b and tell the orchestrator.
