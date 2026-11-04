# M6-17 E2E: fake provider, offline, identify flow
Owner: mivo-frontend
Depends on: M6-14, M6-15, M6-16
Files/areas: `web/apps/web/e2e/**`, `scripts/fake-provider/**` (small Go or Node server with the recorded fixtures)

## Goal
Core M6 flows verified end to end without the internet.

## Acceptance criteria
- [ ] The E2E harness starts a fake TMDB/image server from `testdata/providers/**` and points
      the server at it by the test-only base-URL option.
- [ ] Flow 1: add a library, scan, posters and titles appear from the fake provider, detail
      page shows the TMDB attribution.
- [ ] Flow 2: an ambiguous title stays "Not matched"; the filter finds it; Fix match picks the
      candidate; the item turns matched and a locked field (title set via fixture) is unchanged.
- [ ] Flow 3: Remove match drops the provider poster and keeps local data; a rescan does not
      re-match it.
- [ ] Flow 4: fake server down → library and playback still work, status shows "offline",
      recovery after the server returns.
- [ ] Flow 5: online switched off in Settings → fake server receives zero requests.
- [ ] Tests run with `pnpm -C web e2e`; CI has no network access beyond localhost.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Known existing e2e breakage (BACKLOG M3-19m) must be resolved or tracked before this task is
accepted; do not widen the scope to fix unrelated specs.
