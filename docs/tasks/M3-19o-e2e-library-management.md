# M3-19o E2E: empty state → add library → scan → edit → remove
Owner: mivo-frontend
Depends on: M3-19l, M3-19n, M3-18 (owns `e2e/support/server.ts` changes before)
Files/areas: `web/apps/web/e2e/library-management.spec.ts` (new), `web/apps/web/e2e/support/**`
(`server.ts`: start without libraries and with `MIVO_BROWSE_ROOTS`; a helper that prepares a
browse root with corpus copies, a hidden and a system folder)

## Goal
The core flow of library management works in a real browser against the real server binary.

## Acceptance criteria
- [ ] The server starts with zero libraries and a browse root holding two folders with corpus
      videos plus `@eaDir` and `.hidden` folders.
- [ ] Flow: Home shows the empty state → **Add library** opens Settings › Libraries with the
      dialog → choose Movies, browse into the root (the system and hidden folders are not
      listed), select both folders, keep the prefilled name → Save → the library appears in the
      sidebar, the scan finishes, items from both folders are listed → Edit: rename and remove
      one folder → Save → that folder's items disappear after the scan → Remove with
      confirmation → empty state again.
- [ ] Negative: choosing a folder already used by another library shows the `folder_overlap`
      message on that folder.
- [ ] Accessibility: axe check on the settings page and both dialogs (existing helper).
- [ ] Runs in `pnpm -C web e2e` with the existing setup (web build + server binary on a free
      port); no new network access.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web e2e` green twice in a row (no flakes).

## Notes
- The browser connects from loopback, so the management guard admits it; the guard's refusal
  paths are covered by the Go tests (M3-19k, M3-19l), not here.
- Keep the corpus copies small (two short clips) so the scan finishes within the test timeout.
