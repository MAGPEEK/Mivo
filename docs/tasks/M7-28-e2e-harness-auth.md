# M7-28 E2E harness: first-run setup and signed-in state
Owner: mivo-frontend
Depends on: M7-01
Files/areas: `web/apps/web/e2e/globalSetup.ts`, `web/apps/web/playwright.config.ts`, new `web/apps/web/e2e/support/auth.ts`

## Goal
All existing Playwright specs keep passing after sign-in is switched on, without changes to
each spec.

## Acceptance criteria
- [ ] The global setup, after starting the server binary, calls `GET /api/v1/setup`; when
      required it performs setup with a fixed test admin, signs in through the UI-independent
      API and saves a Playwright `storageState` used by default; on a server without
      sign-in (404 before M7-01, 501 stub until M7-21) it saves an empty state.
- [ ] A helper creates further accounts/profiles over the API for M7-38 (no UI clicks).
- [ ] `pnpm -C web e2e` is green on the current `main` (pre-M7 server) and on a branch with
      M7-25 (verified by the tester when M7-25 is tested).
- [ ] Gates: `pnpm -C web lint` and `pnpm -C web test` pass.

## Notes
Generate the corpus before e2e (testers-run-e2e rule). Cookies are same-origin on the free
port; use `request.newContext({ baseURL })` and `storageState()`.
