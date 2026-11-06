# M7-17 App: user and profile administration
Owner: mivo-backend
Depends on: M7-04, M7-05
Files/areas: new `internal/app/users/`

## Goal
Admins manage accounts, access and limits; account owners manage their managed profiles
(US-10, US-11, US-20, US-22 without single titles).

## Acceptance criteria
- [ ] Admin-only (viewer admin flag, else `domain.ErrForbidden`): list, create (with primary
      profile, initial password hashed), update (name, role, disabled, library access, primary
      profile limit), delete, reset password (revokes the account's sessions through a small
      `SessionRevoker` port).
- [ ] Last-admin rule and unique names enforced (`domain.ErrConflict`), checked through the
      store transaction (M7-08), not only in memory.
- [ ] Profiles: admin or owning account may create (≤ 10 managed), update (name, avatar, PIN
      set/clear, limit, library subset), delete managed profiles; the primary cannot be deleted;
      a subset outside the account's access is refused with `domain.ErrInvalid`.
- [ ] Changing access, limit, disabled state or a PIN evicts the affected sessions from the
      resolve cache (port to `app/auth`, 1 method).
- [ ] `ResetPassword` is also callable without a viewer through an explicit `System` entry used
      only by the CLI (M7-26), documented as such.
- [ ] Tests: fakes (`go test ./internal/app/users/`): non-admin refused, last admin, profile
      cap, owner editing another account's profile refused, cache eviction calls.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Runs in parallel with M7-15 (different package). Avoid importing `app/auth`; declare the
revoke/evict ports here and let `cmd/mivo` wire the auth service into them.
