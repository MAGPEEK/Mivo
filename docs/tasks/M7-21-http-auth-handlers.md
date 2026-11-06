# M7-21 HTTP: setup, sign-in, me, profile and devices handlers
Owner: mivo-backend
Depends on: M7-16, M7-20
Files/areas: `internal/adapters/http/restapi/auth*.go` (replaces `auth_stub.go`), one field each in `restapi/api.go` and `restapi/options.go`

## Goal
The M7-01 operations work end to end on top of `app/auth`.

## Acceptance criteria
- [ ] Setup and sign-in set the cookie (`HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` on TLS,
      `Max-Age` = absolute lifetime) for `client=web` and return the token in the body only for
      `tv|cli`; sign-out clears the cookie and deletes the session.
- [ ] `me`, profile select/clear, password change and own devices map the use-case errors to the
      codes of M7-01 (`invalid_credentials`, `pin_invalid`, `profile_locked` with
      `retryAfterSeconds`, `409` setup done).
- [ ] Device name defaults to a short label from User-Agent (browser + OS, ≤ 60 runes); the raw
      User-Agent is not stored.
- [ ] Tests: handler tests with fake services (`go test ./internal/adapters/http/restapi/ -run Auth`):
      cookie attributes, token only for non-web clients, error codes, password never echoed or
      logged (request log test).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Parallel to M7-22/M7-23; they all add one line to `api.go`/`options.go` – merge one at a time.
