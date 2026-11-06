# M7-24 Secret hygiene: redaction words and database file modes
Owner: mivo-backend
Depends on: none
Files/areas: `internal/platform/logging/` (redaction), `internal/adapters/sqlite/db.go` (file creation), `docs/tasks/BACKLOG.md` (close rows)

## Goal
Passwords, PINs, tokens and cookies never reach a log line, and the database that now holds
password hashes is not world-readable.

## Acceptance criteria
- [ ] The log redaction covers keys and words `password`, `pin`, `passcode`, `token`, `bearer`,
      `session`, `cookie`, `authorization`, `secret`, `otp`, `pwd` (case-insensitive, also as
      parts like `newPassword`, `currentPassword`, `X-Session`), and `Authorization`/`Cookie`/
      `Set-Cookie` header values in access and request logs.
- [ ] Database, WAL and SHM files are created with mode `0600` on Unix (pre-create or umask
      inside the open path); Windows behaviour documented as ACL-inherited.
- [ ] BACKLOG rows "logging … M7 (PINs)" and "sqlite DB/WAL/SHM files created 0644 … M7" are
      closed with this task id.
- [ ] Tests: `go test ./internal/platform/logging/ ./internal/adapters/sqlite/ -run 'Redact|FileMode'`
      (file-mode test skipped on Windows).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Check the existing redaction from M1-05b first and extend its list; do not add a second
mechanism.
