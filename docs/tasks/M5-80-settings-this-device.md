# M5-80 Settings for this device
Owner: mivo-frontend
Depends on: M5-21, M5-03 (prototype approved)
Files/areas: `web/apps/web/src/features/settings/**` (new `components/DeviceSettingsPage.tsx`, `hooks/useDevicePreferences.ts`, `lib/preferenceStore.ts`), routes `/settings/app/*`, top-bar and sidebar settings entries (SH-05, SH-18), locales

## Goal
A page for per-device preferences (AD-01, AD-03): language, hero on/off, default view and card
size, reset sidebar layout, auto play, default speed, clear recent searches; M4 quality and
subtitle entries later.

## Acceptance criteria
- [ ] One typed `preferenceStore` (versioned local storage, defaults, schema validation) is the
      only storage used by M5-21/26/33/50/64; those tasks read through its hooks.
- [ ] Settings entry opens `/settings/app` (M7 changes admins to `/admin`); direct URL works;
      sections as in the prototype; every change applies immediately and persists across reload.
- [ ] Language switch changes the UI language live (en/de) and persists.
- [ ] Reset buttons confirm via `ConfirmDialog`.
- [ ] Tests: store table tests (corrupt data, unknown keys, migration), page tests, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
If other M5 tasks land first with their own keys, this task moves them into the store and
removes the duplicates (Boy Scout), keeping key names stable for existing users.
