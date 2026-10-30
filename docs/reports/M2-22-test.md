# M2-22 test report: libraries in the sidebar, library grid and scan button

Status: PASS (round 4; the round 2 sections below stay for history)
Scope: `web/apps/web/src/features/library/**`, `shared/ui/{Sidebar,Card,Badge}`, `app/layout/AppLayout`,
`src/test/{fakeApi,libraryFixtures,axe}.ts`. M2-23 (`features/item`) is also uncommitted; the repo-wide
gates are green for both, so no failure is attributed to either feature.
Go gates: not applicable (web-only task; the Go changes in the working tree belong to other tasks).

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| Install | `pnpm -C web install --frozen-lockfile --offline` | "Lockfile is up to date", exit 0 |
| ESLint | `pnpm -C web lint:eslint` (`--max-warnings=0`) | exit 0 |
| Layer rules | `pnpm -C web lint:deps` (dependency-cruiser) | "no dependency violations found (210 modules, 476 dependencies cruised)" |
| Prettier | `pnpm -C web lint:format` | "All matched files use Prettier code style!" |
| Types | `pnpm -C web lint:types` (`tsc -b`) | exit 0 |
| Vitest + coverage | `pnpm -C web test -- --coverage` | 52 files, 873 tests passed; Statements 100 % (954/954), Branches 100 % (552/552), Functions 100 % (328/328), Lines 100 % (922/922); thresholds (global 70, player-core 90) met |
| Build | `pnpm -C web build` | exit 0 |
| E2E run 1 | `pnpm -C web e2e` | 10 passed (9.1 s) |
| E2E run 2 | `pnpm -C web e2e` | 10 passed (10.7 s); no flakiness, so no `playwright-report` kept |

Code hygiene grep over `features/library`, `shared/ui`, `app/layout`, `test`: no `TODO`, `FIXME`, `any`
or `eslint-disable`; no hex colour literals; en/de key sets are identical (plural suffixes normalised) in
`features/library/locales` and `shared/i18n/locales`.

## Real-browser checks

Setup: scratch binary `go build -tags webui` (built into the session scratchpad, not the repo) started with
isolated `MIVO_*` config and dirs, `MIVO_LIBRARIES` pointing at a temp folder; headless Chromium via
Playwright plus `@axe-core/playwright` (including colour contrast, unlike the jsdom tests). The temp
directories were removed afterwards and `git status` shows no stray files.

### Against the real server (library/scan/item endpoints are 501 stubs)

- `GET /api/v1/libraries`, `…/{id}`, `…/{id}/items`: HTTP 501 each.
- Sidebar shows "Libraries could not be loaded"; home still renders its heading.
- Collapsed: the message is `sr-only` (1x1 px box).
- `/libraries/<id>` shows the alert "The library could not be loaded. Something went wrong. Try again later."
  with one "Try again" button.
- axe (serious/critical, with contrast): none for home with sidebar expanded, collapsed, and for the library
  error page. No uncaught page errors.

### Library page against a mocked API (Playwright route interception on the real built app)

All checks passed except two, which were mistakes in my scratch script, not product defects (see below).

- Sidebar: two links (Movies, Shows), unavailable library announced as "Shows, Unavailable".
  axe: none, sidebar expanded and collapsed.
- Library page: heading "Movies", "5 videos", `ul` labelled "Videos" with 3 cards, each `MKV · H.264 · AAC` and
  duration (`1:31:01`, `1:32:02`, `1:33:03`). axe: none.
- Load more, failing (HTTP 500): while loading the button is `aria-busy=true`/`aria-disabled=true` with
  focus kept; a second click sends no request (2 -> 2 item requests); after the failure the 3 cards stay,
  inline alert "More videos could not be loaded." is shown, focus stays on the button (now "Try again"), axe
  none. Retry success: 5 cards, focus on the link of the first new card ("Item 4", `/items/aaaaaaaa-…04`),
  button gone.
- Card click navigates to `/items/<itemId>`.
- Scan: button gets `aria-busy=true` + `aria-disabled=true` and keeps focus; polite `role=status` shows
  "Scanning the library folder…"; a second click while running sends no second POST (posts=1); poll
  intervals 2020, 2008 and 2012 ms; result "Last scan: 3 added, 1 updated, 2 removed, 29 unchanged,
  1 failed, 4 skipped."; polling stops after the finish (5 -> 5 requests in 4.5 s); button usable again;
  item count refreshed 5 -> 7; axe none while running and after finishing.
- Polling stops on navigation: left a running scan via the "Shows" sidebar link, 0 further requests for
  the old library in 5.2 s. Unavailable library page shows the "Folder unavailable" badge; empty library shows
  "Run a scan to find the videos…"; axe none.
- 404 while polling (page had old "running" data): "This library does not exist." with "Back to home" link
  shown within the poll interval; no further requests in 5.2 s; axe none.
- Empty library list: sidebar says "No libraries yet" and "Add folders under libraries in config.yaml or
  set MIVO_LIBRARIES, then restart Mivo."; home text points to `config.yaml`; axe none.
- German locale: "Bibliothek scannen" and "… Videos" render. No uncaught page errors.

Script artefacts (not defects): my first duration regex expected `1:30:` while the fixtures give `1:31:01`
(the output shows chips and durations correct); and the last "poll" delta of 11 ms is the intended
`invalidateQueries` refetch right after the scan finished, not a poll.

## Acceptance criteria

1. Sidebar "Libraries" section, one link per library, icon by kind, unavailable marked, TanStack Query hook in
   `features/library` passed as props: met. `useSidebarLibraries` -> `Sidebar libraries` prop; browser check
   above; `AppLayout.test.tsx` "links every configured library with the icon of its kind".
2. `/libraries/$libraryId`: heading, count, grid of cards (2:3 placeholder, initial, title, duration, chip,
   unavailable greyed with label), Load more via `nextCursor` (`useInfiniteQuery`): met (browser check;
   `LibraryItemGrid.tsx`, `libraryQueries.ts:libraryItemsQueryOptions`).
3. Scan button: POST, poll every 2 s while running (named constant `scanStatusPollIntervalMs`), counts when
   finished, translated error for `failed` (`root_unavailable` and generic): met (measured ~2.0 s intervals;
   `LibraryPage.test.tsx` scan status `it.each` incl. `root_unavailable`/generic). "The button is disabled
   while running": met functionally with a justified deviation, see observation 1.
4. Empty states: no libraries (config.yaml / `MIVO_LIBRARIES`, product name from brand), empty library "Run a
   scan": met (browser check; variable and name derive from `productId`/`productName` in `useSidebarLibraries`).
5. Card click navigates to `/items/$itemId`: met (browser and component test).
6. Strings in `features/library/locales/{en,de}.json` only, German complete: met. Key sets identical;
   probe error codes `probe_failed`/`probe_timed_out` and unknown-code fallback (`errors.generic`) are
   translated (orchestrator note M2-02); sidebar section labels and home empty text live in
   `shared/i18n` as before.
7. Accessibility: list with labelled links, scan button `aria-busy`, polite live region, axe: met. Component
   tests assert no serious/critical axe findings; the real-browser axe runs (with contrast) are clean for
   sidebar expanded/collapsed, grid, running scan, empty, unavailable, not-found and error states.
8. Tests (sidebar links, grid paging, scan polling with fake timers, empty states, error state), coverage
   thresholds: met (873 tests, 100 % on all four metrics).
9. Gates lint, test, build, e2e (existing specs green): met (all exit 0, e2e 10/10 twice).
10. Notes: no virtualisation (page size 60); chips use `containerLabel`/`codecLabel` from `@mivo/player-core`;
    no hex literals (ESLint): met. `pnpm-lock.yaml` unchanged (install with `--frozen-lockfile` succeeded;
    axe-core is loaded through `@axe-core/playwright` in `src/test/axe.ts`).
11. Orchestrator notes (probe error codes, namespace registration in `app/i18n.ts`, route params as props):
    met.

Round 2 fixes, all verified: failed Load more and failed background refetch keep the grid with an inline
error and retry (browser and component tests); focus target chosen by item id; polling stops on 4xx and a
404 shows not-found even with cached data; robust `select` for a malformed list (`AppLayout.test.tsx`);
`aria-disabled` while busy; number formatting (`{{count, number}}`); home text points to `config.yaml`.

## Observations (non-blocking)

1. The AC says "disabled while running"; the implementation uses `aria-disabled="true"` plus `aria-busy`
   for the scan button and "Load more". Judgement: acceptable and better for accessibility. A natively
   `disabled` button drops keyboard focus (focus would jump to `body` when the click starts a scan or load);
   `aria-disabled` is announced as disabled, is styled as disabled, and the handler ignores clicks, which I
   verified in the browser (no second POST, no second page request). The architect should adjust the AC
   wording to "`aria-disabled`" so the task file matches the code.
2. No E2E spec covers the library UI against the real server because its library endpoints are still
   501 stubs; the AC only requires the existing specs to stay green. A real-server library flow belongs to
   the task that wires the endpoints.
3. The home placeholder "Your library is empty" is shown regardless of configured libraries; this predates the
   task and is the home feature's concern.

## History

- Round 1: no report on file (task folder had none); fixes listed above are from the orchestrator's round-2
  summary.
- Round 2: PASS.
- Round 3: no report on file (fixes are listed in the round-4 request).
- Round 4: PASS (see the round-4 section below).

## Round 4

Scope: the changes of rounds 3 and 4 (`cancelQueries` with `exact: true`; stale scan-start error via a
`startedAt` snapshot; `useNewPageFocus` anchored on the last item id; any 4xx = gone in `useLibrary`; test
timeouts `asyncUtilTimeout` 5 s in `src/test/setup.ts` and `testTimeout` 15 s in `vitest.config.ts`; the
narrow-screen sidebar overlay `app/layout/{useNarrowScreen,useSidebarOverlay,useSidebarState}`; new
`e2e/layout.spec.ts`). Go gates: not applicable except `go vet -tags webui` / `go build -tags webui`
against the embedded build.

Isolation: all gates ran in a detached git worktree of the then-HEAD `5e64b56` with exactly the 50 files of
`m2-22-files.txt` copied in (`pnpm-lock.yaml` and the api-client schema untouched; `git status` there showed
only those files). The worktree, the scratch binary and the scratch server directory were removed afterwards
(`git worktree prune`; the long `node_modules` paths needed `cmd /c rd /s /q \\?\...`). Main HEAD has moved to
`265e4bf` meanwhile; that is unrelated to this task.

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Install | `pnpm -C web install --frozen-lockfile --offline` | exit 0 ("Done in 20.1s") |
| ESLint | `pnpm -C web run lint:eslint` (`--max-warnings=0`) | exit 0 |
| Layer rules | `pnpm -C web run lint:deps` | "no dependency violations found (215 modules, 488 dependencies cruised)" |
| Prettier | `pnpm -C web run lint:format` | "All matched files use Prettier code style!" |
| Types | `pnpm -C web run lint:types` (`tsc -b`) | exit 0 |
| Vitest + coverage | `pnpm -C web test -- --coverage` | 53 files, 890 tests passed; Statements 100 % (1010/1010), Branches 100 % (584/584), Functions 100 % (342/342), Lines 100 % (976/976) |
| Build | `pnpm -C web build` | exit 0 |
| Go vet (embedded) | `go vet -tags webui ./...` | exit 0 |
| Go build (embedded) | `go build -tags webui ./cmd/mivo` | exit 0 |
| E2E run 1 | `pnpm -C web e2e` | 17 passed (11.1 s): shell 6, accessibility 3, layout 8 |
| E2E run 2 | `pnpm -C web e2e` | 17 passed (11.0 s) |

Hygiene over `features/library`, `app/layout`, `shared/ui`, `test`, `e2e/layout.spec.ts`: no `TODO`, `FIXME`,
`eslint-disable`, `any`, hex literal. en/de key sets identical (37/37 in `features/library/locales`, 17/17 in
`shared/i18n/locales`, plural suffixes normalised).

### Flake proof

- `vitest run src/features/library src/app/layout` 20x sequentially (130 tests each): 20/20 green, 0 failures
  (2 min 55 s total).
- The same selection in 4 concurrent vitest processes: 4/4 green (130 tests each).
- Extra: the whole `apps/web` suite (381 tests) in 4 concurrent processes: 4/4 green.

### Real browser (built app served by the `-tags webui` binary, library endpoints mocked with `page.route`)

Playwright chromium plus `@axe-core/playwright` (colour contrast included); the mock answers 404/500 as
`application/problem+json` like the server. Scratch script in the session scratchpad, not in the repo.

Narrow, 320 px and 375 px (both identical, all green):
- First visit: sidebar closed (`aria-expanded=false`, 64 px icon rail; `main` starts at x=64), h1 uncovered
  (`elementFromPoint`), no horizontal scroll.
- Opening with the toggle: 240 px overlay above the page (`z-index` 30, `main` does not move), no horizontal
  scroll, nothing written to `localStorage`; reload while open starts closed.
- Escape closes and focus is on the toggle. Tab out of the nav (from a library link; 2 Tab presses) closes it
  and the newly focused element is uncovered. Shift+Tab from the toggle lands on the skip link, closes the
  overlay, the skip link is visible and uncovered. A click on the page closes it.
- Library link: closes the overlay, URL `/libraries/<id>`, h1 "Movies", first card and h1 uncovered, no
  horizontal scroll. "Shows" is announced as "Shows, Unavailable".
- Skip link: first Tab stop; Enter focuses `main#main-content` with the overlay closed, after the overlay was
  closed by moving focus to the skip link (also opened by keyboard: 2nd Tab stop is the toggle, Enter opens),
  and on a 1280 px window.
- No uncaught page errors.

Wide, 1280 px:
- First visit expanded; sidebar (x 0..240) sits beside `main` (x=240); no horizontal scroll; h1 uncovered.
- Collapsed and expanded states each persist across reloads; Escape and a click on the page do not collapse
  the wide sidebar.
- Resize: 767 px closes it, 768 px restores the preference (expanded); 600 px closed, opens as an overlay,
  the stored preference stays `false` (not overwritten); widening forgets the overlay and restores the
  preference; narrowing again starts closed; a collapsed preference is likewise restored after an overlay
  at 600 px and survives a reload.

axe (serious/critical incl. contrast; also no minor/moderate findings), en and de, at 320, 375 and 1280 px:
library page with default sidebar, with the overlay open (narrow) or collapsed (wide), and home: 18 of 18
clean. Extra: de running and finished scan, en unavailable/empty library at 320 px: clean. German strings
verified ("Bibliothek scannen", "Mehr laden", `html lang=de`).

Library page flows (re-check of round 2): 60 cards on the first page, "Load more" appends 10 and moves focus to
the link of item 61, button gone on the last page; scan: `aria-busy` + `aria-disabled` and focus kept, a
forced second click sends no second POST (posts=1), polite status, poll intervals 2015 and 2014 ms, polling
stops after the finish (4 -> 4 requests in 4.5 s), button usable again; 404 while polling shows "This library
does not exist." with "Back to home" and stops polling (4 -> 4); failed "Load more" keeps the 60 cards, shows
the inline error and keeps focus on "Try again"; a failed scan start (503) shows "could not be started" and
the message is gone after a successful retry.

Script artefacts (mine, not product defects): the closed state at narrow width is a 64 px icon rail rather
than a hidden nav, so my "library link not visible" expectation was wrong; Playwright treats
`aria-disabled="true"` as not actionable and waited until the scan ended, so the second click needed
`force: true`; my first 404 mock used `application/json`, which the client correctly maps to a generic error
(the server sends `application/problem+json`).

### Acceptance criteria (re-checked)

1. Sidebar "Libraries" section, links by kind icon, unavailable marked, hook in `features/library` passed as
   props: met (browser; `AppLayout.test.tsx`, `Sidebar.test.tsx`).
2. `/libraries/$libraryId` heading, count, card grid, Load more via `useInfiniteQuery`: met (browser flow).
3. Scan button: POST, 2 s polling while running (`scanStatusPollIntervalMs`), counts, translated errors,
   busy while running: met (intervals ~2.01 s); `aria-disabled` instead of `disabled` as per the round-2
   orchestrator note.
4. Empty states (no libraries with `config.yaml`/`MIVO_LIBRARIES`/brand name; empty library "Run a scan"): met
   (browser, unavailable empty library at 320 px; no-libraries text verified in round 2, unchanged).
5. Card click navigates to `/items/$itemId`: met (component tests; round-2 browser check, unchanged).
6. Strings in `features/library/locales`, German complete: met (key sets identical, German verified).
7. Accessibility (list, `aria-busy`, polite live region, axe): met (component and browser axe, 0 findings).
8. Tests and coverage thresholds: met (890 tests, 100 % on all four metrics).
9. Gates lint, test, build, e2e: met (e2e 17/17 twice, existing specs green plus the new `layout.spec.ts`).
10. Notes (no virtualisation, chip labels from player-core, no hex): met; `pnpm-lock.yaml` unchanged.
11. Orchestrator notes (probe error codes, namespace registration, route params as props): met.

### Observations (non-blocking)

1. Race of a few milliseconds on the scan button: with a Playwright click on a page where "Load more" had
   been used before, the button still showed "Scan library" / `aria-disabled=false` 4 ms after the click had
   already sent its POST (the TanStack Query `isPending` notification is delivered on a timer). A second
   click within that window would start a second POST; I could not provoke it with in-page clicks 0, 10 and
   100 ms apart (always one POST). A person cannot click twice within 4 ms, so this is not a defect, but a
   synchronous ref guard in `useLibraryScan.start` would remove the window if automation ever clicks twice
   in a row.
2. Observations 1-3 of round 2 stand (AC wording "disabled" vs `aria-disabled`; no real-server E2E for the
   library UI while its endpoints are 501 stubs; home placeholder text).
