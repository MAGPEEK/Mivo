# M7-08 SQLite: migration 00011 and account/profile store
Owner: mivo-backend
Depends on: M7-04, M7-05
Files/areas: `internal/adapters/sqlite/migrations/00011_accounts.sql`, `internal/adapters/sqlite/queries/accounts.sql`, generated `sqlcgen`, new `internal/adapters/sqlite/account_store*.go`

## Goal
Accounts, profiles, library access and sessions have a schema, and accounts and profiles can be
stored and read with the invariants enforced in transactions.

## Acceptance criteria
- [ ] Migration `00011` creates `users` (unique `name_key`), `profiles` (FK users `ON DELETE
      CASCADE`, one primary per user via a partial unique index, `pin_hash` nullable,
      `max_age` nullable, `hide_unrated`, `all_libraries`), `user_libraries` and
      `profile_libraries` (FK to `libraries ON DELETE CASCADE`), and `auth_sessions`
      (token hash unique, user and profile FKs, device fields, timestamps, `remember`) per
      ADR 0032 §1, §3, §7; goose down drops them.
- [ ] Store methods: create the first admin only while `users` is empty (one transaction,
      `domain.ErrConflict` otherwise); create/update/delete account (with primary profile);
      get by id and by name key; list with profile count and last activity; set password hash;
      profile CRUD incl. PIN hash; read effective access and limit inputs for a profile.
- [ ] The last-admin rule is re-checked inside the write transaction (no race between two admins
      demoting each other); duplicate names map to `domain.ErrConflict`.
- [ ] Tests: integration tests against a temp-dir SQLite (`go test ./internal/adapters/sqlite/ -run Account`):
      migration up/down, first-admin race (two goroutines, exactly one wins), cascade on user
      delete, library delete removes access rows, last-admin conflict.
- [ ] Gates: lint + layer rules + coverage thresholds pass; `sqlc generate` leaves the tree clean.

## Notes
Store types implement the ports M7-15/M7-17 declare; agree method names via those task files
(small interfaces, accept domain types from M7-04/M7-05). Sessions are only created here
(schema); queries for them are M7-09. Never log hashes.
