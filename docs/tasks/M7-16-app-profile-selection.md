# M7-16 App: profile selection, PIN lockout, devices
Owner: mivo-backend
Depends on: M7-15
Files/areas: `internal/app/auth/` (new files for profiles and devices)

## Goal
A signed-in device picks a profile (with PIN when set), the lockout stops PIN guessing, and
users see and revoke their devices (US-01…03, US-25).

## Acceptance criteria
- [ ] `Me(ctx)` returns account, current profile and the picker list (has PIN, primary, admin).
- [ ] `SelectProfile(ctx, profileID, pin, remember)`: the profile must belong to the session's
      account; PIN verified via the hasher; 5 wrong attempts lock the profile 5 minutes
      (`domain.ErrRateLimited` with retry-after), success resets the counter;
      `remember` default false for PIN profiles; a non-remembered selection lapses after
      4 hours without requests (checked in `Resolve`).
- [ ] `ClearProfile(ctx)`; resolving a session without a profile yields a viewer flagged
      "no profile" that catalog use cases reject with a dedicated error mapped to
      `profile_required` (M7-20).
- [ ] `ListDevices(ctx)` (own), `ListAllDevices(ctx)` (admin), `RevokeDevice(ctx, id)` (own or
      admin; evicts the cache; revoking the current device signs out).
- [ ] Tests: fakes (`go test -race ./internal/app/auth/`): wrong PIN ×5 then lock, unlock after
      the window, PIN of another account's profile refused, remember expiry with a fake clock,
      revoke scoping.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Define the "no profile" error next to the use case (wrapping `domain.ErrForbidden`) so the HTTP
mapper can tell it apart by `errors.Is`. Named constants for 5 attempts, 5 minutes, 4 hours.
