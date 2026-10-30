# M1-17 Playwright E2E for the walking skeleton
Owner: mivo-frontend
Depends on: M1-15, M1-16
Files/areas: `web/apps/web/playwright.config.ts`, `web/apps/web/e2e/**`,
`web/apps/web/package.json` (`e2e` script and devDependencies)

## Goal
One command proves the whole stack end to end: built web app embedded in the Go binary,
served with a real SQLite database, rendering the shell with live server data.

## Acceptance criteria
- [ ] `pnpm -C web e2e` builds the web app and then runs Playwright. A global setup builds
      the server with `go build -tags webui ./cmd/mivo` into a fresh temp directory and starts
      it on 127.0.0.1 with a free port, `MIVO_CONFIG_DIR`, `MIVO_PATHS_DATA`,
      `MIVO_PATHS_CACHE` and `MIVO_PATHS_TRANSCODE` pointing into that directory. It waits for
      `/api/v1/health` (see the orchestrator decision below).
- [ ] Specs (Chromium):
      1. Shell renders: navigation landmark, search box, home heading, empty-state text.
      2. Sidebar footer version equals `version` from a direct `GET /api/v1/system/info`.
      3. `Control+K` focuses the search box.
      4. Sidebar toggle collapses and expands (`aria-expanded`). The state survives a reload.
      5. Deep link `/does-not-exist` loads the app (server SPA fallback) and shows the
         not-found view.
      6. `@axe-core/playwright` reports no `serious` or `critical` violations on home.
- [ ] Works from PowerShell on Windows. Env is set by the global setup in Node (the
      `spawn` environment), not shell syntax.
- [ ] Traces and screenshots on failure go to git-ignored `test-results/` /
      `playwright-report/`.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test` and `pnpm -C web e2e` pass.

## Notes
- Installing the browser (`pnpm -C web exec playwright install --only-shell chromium`)
  downloads binaries. Ask the orchestrator for user approval first.
- Requires Go on PATH. If Go is missing, fail with a clear message, not a timeout.
- A free port chosen per run avoids clashing with a dev server on 8460 or a parallel run.
- Keep E2E deterministic: no reliance on the developer's real config directory.

## Review carry-over from M1-15 (APPROVE) – fix while touching web/
- `useSidebarState`: remove the unused storage parameter (or use `null` for in-memory) and fix the doc.
- `systemInfoQuery`: reword the `staleTime: Infinity` comment ("a stale version until the page is reloaded is acceptable").
- Collapsed sidebar: show a small token-coloured error indicator so sighted users see "Server unreachable".
- `createApiClient`: reject base URLs with credentials, query or hash; tests.
- Nits: `isComposing` guard and `event.code === "KeyK"` fallback for Ctrl/Cmd+K; less DOM-coupled Sidebar footer test; webOS `cause` note on the TypeError.

## Orchestrator decision (2026-10-30) – replaced the original webServer/go run/fixed port wording
- E2E runs against the real binary built with `go build -tags webui` into a fresh temp dir by a
  Playwright global setup, started on 127.0.0.1 with a free port and a fully isolated environment
  (config/data/cache/transcode, profile dirs, no inherited `MIVO_*`), ready when `/api/v1/health`
  answers; teardown stops the server and removes the temp dir. Reason: `webServer` starts before
  global setup and is killed hard, so build-before and clean teardown are not possible with it.
