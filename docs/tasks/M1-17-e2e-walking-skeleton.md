# M1-17 Playwright E2E for the walking skeleton
Owner: mivo-frontend
Depends on: M1-15, M1-16
Files/areas: `web/apps/web/playwright.config.ts`, `web/apps/web/e2e/**`,
`web/apps/web/package.json` (`e2e` script and devDependencies)

## Goal
One command proves the whole stack end to end: built web app embedded in the Go binary,
served with a real SQLite database, rendering the shell with live server data.

## Acceptance criteria
- [ ] `pnpm -C web e2e` builds the web app and then runs Playwright. Its `webServer` starts
      `go run -tags webui ./cmd/mivo` from the repository root with
      `MIVO_CONFIG_DIR`, `MIVO_PATHS_DATA`, `MIVO_PATHS_CACHE` and `MIVO_PATHS_TRANSCODE`
      pointing to a fresh temp directory and `MIVO_SERVER_PORT=18460`. It waits for
      `/api/v1/health`.
- [ ] Specs (Chromium):
      1. Shell renders: navigation landmark, search box, home heading, empty-state text.
      2. Sidebar footer version equals `version` from a direct `GET /api/v1/system/info`.
      3. `Control+K` focuses the search box.
      4. Sidebar toggle collapses and expands (`aria-expanded`). The state survives a reload.
      5. Deep link `/does-not-exist` loads the app (server SPA fallback) and shows the
         not-found view.
      6. `@axe-core/playwright` reports no `serious` or `critical` violations on home.
- [ ] Works from PowerShell on Windows. Env is set through the Playwright `webServer.env`
      option, not shell syntax.
- [ ] Traces and screenshots on failure go to git-ignored `test-results/` /
      `playwright-report/`.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test` and `pnpm -C web e2e` pass.

## Notes
- Installing browsers (`pnpm -C web exec playwright install chromium`) downloads binaries.
  Ask the orchestrator for user approval first.
- Requires Go on PATH. If Go is missing, fail with a clear message, not a timeout.
- Port 18460 avoids clashing with a dev server on 8460. Use a named constant in the config.
- Keep E2E deterministic: no reliance on the developer's real config directory.

## Review carry-over from M1-15 (APPROVE) – fix while touching web/
- `useSidebarState`: remove the unused storage parameter (or use `null` for in-memory) and fix the doc.
- `systemInfoQuery`: reword the `staleTime: Infinity` comment ("a stale version until the page is reloaded is acceptable").
- Collapsed sidebar: show a small token-coloured error indicator so sighted users see "Server unreachable".
- `createApiClient`: reject base URLs with credentials, query or hash; tests.
- Nits: `isComposing` guard and `event.code === "KeyK"` fallback for Ctrl/Cmd+K; less DOM-coupled Sidebar footer test; webOS `cause` note on the TypeError.
