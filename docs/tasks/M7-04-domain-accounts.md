# M7-04 Domain: accounts and profiles
Owner: mivo-backend
Depends on: none
Files/areas: new `internal/domain/account/`, `internal/domain/errors.go`

## Goal
The rules for account names, passwords, roles, profiles, PINs and the last-admin invariant live
in one pure package that every M7 layer uses.

## Acceptance criteria
- [ ] Types `Account` (id, name, role `admin|user`, disabled), `Profile` (id, account id, name,
      primary flag, avatar colour token, has PIN), `Role`; constructors validate and wrap
      `domain.ErrInvalid` with a field name.
- [ ] `NormalizeName` (trim, NFC, 1–40 runes, no control characters) and `NameKey` (case-folded
      key for uniqueness); `ValidatePassword` (8–128 bytes UTF-8); `ValidatePIN` (exactly 4
      ASCII digits).
- [ ] `CheckAdminChange` (or equivalent) refuses deleting, disabling or demoting the last
      enabled admin with `domain.ErrConflict`; `CanManageProfile(actor, profile)` (admin or
      owning account); `MaxManagedProfiles` = 10 as a named constant; the primary profile
      cannot be deleted.
- [ ] `IsAdminViewer(account, profile)` is true only for the primary profile of an enabled
      admin account (ADR 0032 §1).
- [ ] `domain.ErrUnauthenticated` added to `internal/domain/errors.go` with a doc comment.
- [ ] Tests: table-driven `go test -cover ./internal/domain/account/` ≥ 90 %; edge cases for
      Unicode names (NFC vs NFD, case folding `ß`/`İ`), 7/8/128/129-byte passwords, PIN `"0000"`,
      `"123"`, `"12345"`, `"١٢٣٤"` (non-ASCII digits refused).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No I/O, no hashing here (that is M7-07). `golang.org/x/text` is already used for NFC (ADR 0027);
reuse it. Avatar colours are a fixed token list shared with `ui-tokens` names (no hex here).
