# M7-41 Unique profile names per account
Owner: mivo-backend
Depends on: M7-22; must merge before M7-25
Files/areas: `internal/adapters/sqlite/migrations/00015_profile_name_key.sql`, `internal/adapters/sqlite/queries/accounts.sql` (profile statements), `internal/adapters/sqlite/account_store*.go`, regenerated `sqlcgen`, their tests; the name-taken sentinel where M7-17b declared it (no new error)

## Goal
Two profiles of one account can never carry the same name (compared case-insensitively like
account names), also when two requests race: the database refuses the second one and the
client gets `409 name_taken` (ADR 0032 §1) instead of a duplicate tile in the profile picker.

## Acceptance criteria
- [ ] Migration `00015`: `profiles` gains `name_key` (the `account.NameKey` of the name) and a
      unique index on `(user_id, name_key)`; the down migration removes both. Rows written
      before the migration keep a NULL key, so the migration cannot fail on existing data.
- [ ] The store writes `name_key` on every profile insert and rename, including the primary
      profile created with an account and renamed with it.
- [ ] A violated index on profile insert and on profile update returns the existing
      name-taken sentinel (wraps `domain.ErrConflict`); no other constraint error is mapped
      to it (test with a second unique violation, e.g. the one-primary index).
- [ ] `BackfillProfileNameKeys(ctx)` on the account store fills NULL keys; when two legacy
      rows of one account collide, the older keeps the key, the newer stays NULL and is
      reported in the returned count. Idempotent. (M7-25 calls it at start-up.)
- [ ] Tests: integration tests against real SQLite
      (`go test ./internal/adapters/sqlite/ -run 'ProfileName|Migration'`): same name twice,
      differing only in case (`Mia`/`mia`, `Ärzte`/`ärzte`), same name in two accounts
      (allowed), rename onto a sibling's name, rename to the own name with other case
      (allowed), two concurrent inserts (exactly one succeeds), backfill with and without a
      legacy collision, migration up/down on a database with profiles.
- [ ] A handler test in `restapi` shows `409 name_taken` for create and for update when the
      service returns the sentinel (add only if M7-22 did not cover both).
- [ ] Gates: lint + layer rules + coverage thresholds pass; `sqlc generate` diff is clean.

## Notes
- `profiles` has no `name_key` column in migration `00011`, and SQLite cannot case-fold
  Unicode, so the key must come from Go (`account.NameKey`) as for `users.name_key`. That is
  why the column is nullable and the backfill is a store method, not SQL. SQLite treats
  NULLs as distinct in a unique index, so no partial index is needed.
- No released build can create profiles before M7-25 (sign-in is not wired), so legacy rows
  exist only in development databases; the backfill is the safety net, keep it small.
- The use-case check in `app/users` stays (it gives the answer without a write); the index
  closes the race between check and insert.
- Never edit migration `00011`. `00015` merges after `00014`; if M7-13 adds a migration it
  takes `00016` and merges after this task.
