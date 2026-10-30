# M1-15 test report

Status: PASS (round 1)
Scope: web gates only, as requested (no Go gates, no Go files touched; Go agents run in parallel). No Docker, media or Playwright E2E in this task. Nothing staged.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| Install | `pnpm -C web install --frozen-lockfile` | "Lockfile is up to date", exit 0 |
| Lint | `pnpm -C web lint` (eslint `--max-warnings=0` incl. jsx-a11y strict, depcruise, prettier `--check`, `tsc -b`) | exit 0; "no dependency violations found (85 modules, 152 dependencies cruised)"; "All matched files use Prettier code style!" |
| Test | `pnpm -C web test` | 23 files, **264 passed (264)**; Statements 100 % (227/227), Branches 100 % (98/98), Functions 100 % (67/67), Lines 100 % (224/224); thresholds 70 % global / 90 % player-core met |
| Build | `pnpm -C web build` | exit 0; 216 modules; JS 394 kB (126 kB gzip), CSS 13 kB, 7 Inter woff2 files |

Cleanup: the build wrote to `internal/adapters/http/webui/dist` (gitignored via `/internal/adapters/http/webui/dist/`). The directory held nothing but the build, so I removed `internal/adapters/http/webui` again; `web/coverage` removed too.

## Dev-server smoke check (real wiring, no browser)

Stub backend (`scratchpad/stub.mjs`, Node, 127.0.0.1:18460) serves `GET /api/v1/system/info` with a payload matching `SystemInfo` in `api/openapi.yaml`. `MIVO_DEV_API_TARGET=http://127.0.0.1:18460 pnpm -C web dev`:

- `GET http://localhost:5173/` -> 200, index.html with `<div id="root">`, `<script src="/src/main.tsx">`, `<title>Mivo</title>` (injected from brand).
- `GET http://localhost:5173/api/v1/system/info` through the proxy -> 200, `application/json`, `{"productName":"Stub","version":"9.9.9-smoke","serverId":"6f1c...","apiVersion":"1"}`. Proxy and env override work.
- `GET /src/main.tsx` -> 200 (3407 bytes), `GET /src/app/layout/AppLayout.tsx` -> 200 (7613 bytes): modules compile without Vite errors (dev.err empty).
- Both processes stopped afterwards; ports 5173 and 18460 confirmed closed.
- Not done: rendering in a browser (no Playwright browsers installed, per instructions). Rendering is covered by the Testing Library tests; the E2E walking skeleton is M1-17.

## Acceptance criteria

| # | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Landmarks `<nav>` (labelled), `<header>`, `<main>` | met | `Sidebar.tsx:28-30` `<nav aria-label={t("sidebar.label")}>`; `TopBar.tsx:23` `<header>`; `AppLayout.tsx:34` `<main id tabIndex=-1>`. Test `AppLayout.test.tsx:22` finds `navigation` "Main navigation", `banner`, `main` and the h1 inside main |
| 2 | Sidebar: brand name, Home link with `aria-current="page"`, Libraries section "No libraries yet", collapse toggle with `aria-expanded`/`aria-controls`, persisted under key from `brand.productId` | met | `Sidebar.tsx:44-46` (`productName` from `@mivo/ui-tokens`), `:68-75` TanStack `Link` (exact), `:85-95`, `:34-41`. `useSidebarState.ts:7` key `${productId}.sidebar.collapsed`. Tests: `AppLayout.test.tsx:44` (aria-current on `/`), `:52` (absent on other page), `:61`, `:72` (aria-controls equals nav id, starts expanded), `:80` (flip and persisted "true"/"false"), `:95` (restored from storage) |
| 3 | Top bar: labelled search input (visible placeholder, hidden label); Ctrl+K / Cmd+K focuses and prevents default; search does nothing | met | `TopBar.tsx:24-37` (`role="search"` form, `sr-only` label via `htmlFor`, placeholder, `aria-keyshortcuts`, submit prevented). `useSearchShortcut.ts:15-24` (`preventDefault`, `focus`, `select`). Tests `AppLayout.test.tsx:119-131` (Ctrl and Cmd: `fireEvent.keyDown` returns false, activeElement is the search field), `:133` (selects old query), `:144` (plain K ignored), `:154`, `:162` (submit stays on page); unit tests in `searchShortcut.test.ts` |
| 4 | Home: heading plus empty state ("Your library is empty" plus hint) with token colours | met | `HomePage.tsx:13-18` h1 + `EmptyState`; `EmptyState.tsx` uses `border-divider bg-surface text-text text-text-muted text-accent` token classes only. Tests `HomePage.test.tsx:7` |
| 5 | Version footer via `useSystemInfo()` (TanStack Query, client from `ApiClientProvider`): loading skeleton, success, error "Server unreachable" in `role="status"` | met | `ServerVersion.tsx:12` `role="status"` wraps all three states; `Skeleton` (aria-hidden shape + `sr-only` label); `useSystemInfo.ts` uses `useApiClient()`; `systemInfoQuery.ts` `staleTime: Infinity`, `retry: 1`. Tests `ServerVersion.test.tsx:39` (skeleton text "Loading server version"), `:47` (version + exactly one request to `/api/v1/system/info`), `:61` (ApiError 500 -> "Server unreachable", 2 requests = one retry), `:71` (network failure), `useSystemInfo.test.tsx` (data, no refetch on remount, retry once -> `ApiError` `network_error`) |
| 6 | All strings via i18n; en and de complete | met | `en.json`/`de.json` have identical key trees (layout, sidebar, search, home, system, notFound); `locales.test.ts` parity + non-empty test green; hardcoded strings in components: none found by reading (only `productName` from brand, class names) |
| 7 | Logic in hooks (`useSidebarState`, `useSearchShortcut`, `useSystemInfo`); components only render; no raw hex (lint) | met | Hooks located in `app/layout/useSidebarState.ts`, `app/layout/useSearchShortcut.ts`, `features/system/hooks/useSystemInfo.ts`; pure helpers in `shared/lib/{persistentFlag,searchShortcut}.ts`. `grep '#[0-9a-fA-F]{3,8}'` over `web/apps/web/src` finds nothing; eslint hex rule active and lint green |
| 8 | Tests: toggle flips and persists; Ctrl+K focuses; version from fake response; error on `ApiError`; German empty state | met | See rows 2, 3, 5; German: `HomePage.test.tsx:20` ("Startseite", "Deine Bibliothek ist leer", German hint) and `ServerVersion.test.tsx:79` ("Server nicht erreichbar"). Fake API via `ApiClientProvider` (`test/renderApp.tsx`, `test/fakeApi.ts`, real `createApiClient` with fake `fetch`) |
| 9 | Gates lint (jsx-a11y strict, depcruise) and test (thresholds) | met | see Gates |

Notes criteria:
- `Sidebar`, `TopBar`, `EmptyState`, `Skeleton` live in `src/shared/ui`; `grep "features/" web/apps/web/src/shared` finds nothing; depcruise clean. `ServerVersion` is composed in `app/layout/AppLayout.tsx:29` from `features/system` (met).
- Mobile nav, hero, poster rows etc. not stubbed (met). Focus ring: visible focus is handled by the token-based global style (not re-verified visually; `main` suppresses its own outline by design for the skip-link target).

## A11y basics (by reading)

- Sidebar `<nav aria-label="Main navigation">` (de: "Hauptnavigation"): present.
- Skip link `AppLayout.tsx:20-25` (`sr-only`, visible on focus, `href="#main-content"`) targeting `<main id="main-content" tabIndex={-1}>`; test `AppLayout.test.tsx:34` checks href equals main id.
- `role="search"` on the form in the header; input has an associated hidden label and `aria-keyshortcuts="Control+K Meta+K"`.
- Toggle button: `aria-expanded={!collapsed}`, `aria-controls={navigationId}` (the nav id from `useId`), `aria-label`.
- Version footer: `role="status"` in all three states; skeleton exposes its label to screen readers only.
- Collapsed sidebar keeps labels in the accessibility tree via `sr-only` (test `AppLayout.test.tsx:104`).

## Carry-over from M1-14 round 2

| Item | Status | Evidence |
| --- | --- | --- |
| `web/eslint.config.js` hex-rule comment extended ("Fixes (#123)", "#101, #102" after `(` or `,` flagged, belongs in locale files) | met | diff lines 42-49 of the comment block |
| `fontFamily.test.ts` resolves bare `@fontsource-variable/inter` | met | `createRequire(...).resolve("@fontsource-variable/inter")`; test green |

## Carry-over from M1-13

| Item | Status | Evidence |
| --- | --- | --- |
| `requestData`: 2xx except 204 with `data === undefined` throws `ApiError` `unexpected_response` with HTTP status; test added | met | `requestData.ts` (`toEmptyBodyError`, check `status !== 204`); tests: Content-Length 0 and empty body (code `unexpected_response`, status 200, detail "HTTP status 200 without a body."), and 204 resolves to `undefined` |
| `createApiClient` validates non-empty `baseUrl` (`new URL`), clear error; test | met | `client.ts` `assertServerUrl` (TypeError, http/https only, cause kept); `client.test.ts` accepts origin and https with path prefix, rejects "192.168.1.10:8460", "localhost:8460", "/api", blank, `ftp://`, and checks `cause` |
| Nit: `toApiError` JSDoc "caller guarantees !response.ok" | met | `apiError.ts` JSDoc |
| Nit: `pnpm-workspace.yaml` comment (tsc -b and typed tests prove correctness; drift check keeps generation reproducible) | met | diff of `web/pnpm-workspace.yaml` |
| Nit: `ErrorOptions.cause` diagnostics-only, not supported on webOS 6 (Chromium 79) | met | JSDoc on `ApiError` constructor |

## Observations (non-blocking)

- `aria-controls` on the toggle points at the `<nav>` that also contains the toggle (so the whole nav is "controlled", including the button). It satisfies the criterion and the test, but the semantic target could be a narrower content region; a reviewer may weigh it.
- `grep TODO|FIXME` in `web/apps` and `web/packages`: no hits.
- The dev-server smoke check only verifies serving and proxying; no browser rendering was exercised.

## History

- Round 1: PASS. All gates green, 264 tests, 100 % coverage, all criteria and carry-overs met.

REPORT
status: PASS
task: M1-15
files: docs/reports/M1-15-test.md
tests: pnpm -C web install --frozen-lockfile OK; pnpm -C web lint exit 0 (eslint, depcruise 85 modules, prettier, tsc -b); pnpm -C web test 264/264, coverage 100 % (thresholds 70/90 met); pnpm -C web build OK; dev-server smoke (stub backend, proxy for /api/v1/system/info) OK
findings: none blocking; nit: aria-controls on the sidebar toggle targets the nav that contains it
next: hand over to mivo-reviewer; M1-16 (embed web UI) must recreate internal/adapters/http/webui/dist via pnpm build (removed again after this check)
