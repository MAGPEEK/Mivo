# M7-34 Web: devices and revoke (US-25)
Owner: mivo-frontend
Depends on: M7-02, M7-30
Files/areas: new `web/apps/web/src/features/devices/`, `app/router.ts` (two routes), locale files

## Goal
Every user sees the devices signed in to their account and revokes them; admins see all
devices of all accounts with a client filter, as drawn in `admin.html` (Devices).

## Acceptance criteria
- [ ] Own devices page (from the profile menu / own settings): name, client, platform, last
      seen (relative time), address class LAN/remote, "This device" marker, Revoke with
      confirmation; revoking this device signs out.
- [ ] Admin › Devices: same table plus account name and a client-type filter.
- [ ] Tests: Vitest (`pnpm -C web test`): list rendering, revoke other, revoke current → sign-in.
- [ ] Gates: `pnpm -C web lint` and coverage pass.

## Notes
No app-version column in 0.1.0 (only web clients exist); keep the column definition open for
the TV app.
