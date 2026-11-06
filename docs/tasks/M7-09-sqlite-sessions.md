# M7-09 SQLite: session and device store
Owner: mivo-backend
Depends on: M7-08
Files/areas: `internal/adapters/sqlite/queries/sessions.sql`, generated `sqlcgen`, new `internal/adapters/sqlite/session_store*.go`

## Goal
Sessions (which are also the devices) are created, resolved by token hash, touched, listed and
revoked efficiently.

## Acceptance criteria
- [ ] Create session; resolve by token hash returning session, account (incl. disabled) and
      selected profile in one query; touch `last_seen` + address class only when older than
      5 minutes (named constant, enforced in SQL); select/clear profile with `remember`.
- [ ] List sessions of an account and of all accounts (with account name); delete one
      (scoped to an account unless admin), delete all of an account except one, delete
      expired (idle 30 days or absolute 365 days) – returns the deleted token hashes so the
      cache can evict them.
- [ ] Tests: integration tests (`go test ./internal/adapters/sqlite/ -run Session`): resolve
      unknown hash → `domain.ErrNotFound`, touch throttling, expiry purge, revoke scoping
      (user A cannot delete user B's session id), cascade on account delete.
- [ ] Gates: lint + layer rules + coverage thresholds pass; `sqlc generate` leaves the tree clean.

## Notes
Runs in parallel with M7-10/M7-11 (own files); regenerate `sqlcgen/models.go` after merges.
Expiry purge is called by the auth service periodically (M7-15/M7-25), not by a trigger.
