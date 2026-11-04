# M5-23 Activity indicator and settings button
Owner: mivo-frontend
Depends on: M5-16, M5-20
Files/areas: `web/apps/web/src/features/activity/**` (new), top bar wiring in `app/layout`, router, locales

## Goal
A top-bar icon with a badge shows running scans and transcodes; its popover lists them with
progress and links (SH-04); the settings button opens `/settings/app` (SH-05, M5 part).

## Acceptance criteria
- [ ] Polls `/api/v1/activity` (slow when idle, fast while something runs, paused when the tab
      is hidden); no badge when idle.
- [ ] Popover: one row per scan (library, percent) and per session (title, mode); transcodes
      only shown when the API returns session details.
- [ ] Error state (API down) shows a muted icon, no crash.
- [ ] Tests: polling interval logic as pure function; component tests with fake API; axe clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
"Update available" and metadata activity arrive in M7/M6; keep the list model open for new kinds.
