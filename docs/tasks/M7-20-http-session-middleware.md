# M7-20 HTTP: session middleware, setup gate, CSRF, admin guard
Owner: mivo-backend
Depends on: M7-01, M7-15
Files/areas: `internal/adapters/http/server/` (new `auth*.go`, `manageguard.go`, `middleware.go`), `internal/adapters/http/restapi/problem*.go`

## Goal
Every API and media request is authenticated before it reaches a handler; cookie requests are
CSRF-safe; admin operations need an admin viewer and a local peer (ADR 0032 §3, §6, §9).

## Acceptance criteria
- [ ] Middleware reads the brand-derived session cookie or `Authorization: Bearer`, calls the
      resolver port (1 method), and stores the viewer with `access.WithViewer`. Open routes
      exactly as ADR 0032 §6 (health, system info, setup pair, `POST auth/session`, static UI
      and SPA fallback); all other `/api/v1/*` and media routes answer problem `401
      unauthenticated`, or `401 setup_required` while setup is pending.
- [ ] Cookie-authenticated unsafe methods must pass the ADR 0028 same-origin and JSON checks
      (shared helper extracted from `manageGuard`); bearer requests skip them.
- [ ] `manageGuard` keeps the local-peer rule and additionally requires an admin viewer for its
      routes **plus** identify/refresh/match, `settings/metadata`, `/api/v1/users*`,
      `/api/v1/profiles*` of other accounts (owner check stays in the use case) and
      `/api/v1/devices*`; setup is local-peer + same-origin only. The refusal detail no longer
      says "until accounts exist".
- [ ] Problem mapping: `domain.ErrUnauthenticated` → 401, the "no profile" error → 403
      `profile_required`, `ErrRateLimited` → 429 with `Retry-After`; WWW-Authenticate is not
      sent (no browser basic-auth prompt).
- [ ] Tests: `go test -race ./internal/adapters/http/server/ ./internal/adapters/http/restapi/`
      table tests per route class × {no token, cookie, bearer, admin, non-admin, remote peer,
      cross-site Origin, form content type}; encoded-path variants cannot skip the guard.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Order: request id → LAN guard → auth → manage guard → handlers. The resolver is a port declared
here and satisfied by `app/auth.Service.Resolve`. Address class `lan|remote` comes from the
existing LAN classification and is passed to `Resolve`. Close BACKLOG row "m3-19k review
minors" items that this change makes obsolete (Host allow-list stays open; note it).
