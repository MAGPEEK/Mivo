# M7-26 CLI: `admin reset-password`
Owner: mivo-backend
Depends on: M7-25
Files/areas: `cmd/mivo/main.go` (command dispatch), new `cmd/mivo/admin_cli.go`, its test

## Goal
An owner who lost the admin password can set a new one on the host without losing data
(ADR 0006 recovery path, ADR 0032 §6).

## Acceptance criteria
- [ ] `mivo admin reset-password --name <account>` reads the new password from stdin (no echo
      on a terminal, plain read when piped), validates it, stores the hash through
      `app/users` System entry, revokes the account's sessions, and prints a one-line result.
- [ ] Uses the same config/data-dir resolution as `serve` (ADR 0018); works while the server
      runs (SQLite WAL, busy timeout) and when it is stopped; exits non-zero with a clear
      message for an unknown name or an invalid password.
- [ ] `mivo admin list-accounts` prints names and roles (to find the right name).
- [ ] Tests: `go test ./cmd/mivo/ -run Admin` against a temp data dir: reset, unknown name,
      piped input, sessions revoked.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Never accept the password as a flag (shell history). The binary name comes from the brand
package in help texts.
