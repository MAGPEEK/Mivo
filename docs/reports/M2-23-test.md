# M2-23 test report – Web: item detail page with technical chips and Play

Verdict: **PASS** (round 2)
Scope: `web/apps/web/src/features/item/**` and `web/apps/web/src/app/router.test.tsx` (item row removed).
M2-22 (features/library, shared/ui, src/test) was in its own round 2 in parallel and is judged separately.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| ESLint (item + app) | `pnpm exec eslint apps/web/src/features/item apps/web/src/app --max-warnings=0` | exit 0 |
| Prettier (item + app) | `pnpm exec prettier --check apps/web/src/features/item apps/web/src/app` | all files formatted |
| Whole-workspace lint | `pnpm run lint:eslint`, `lint:format`, `lint:deps` | exit 0; depcruise "no dependency violations found (210 modules, 476 dependencies)" |
| Types | `pnpm run lint:types` (`tsc -b`) | exit 0 on the final run. The first run failed in `features/library/components/LibraryItemGrid.tsx(78,18)` (`items.loadMoreFailed` key, M2-22 WIP); gone on the later run. Not an M2-23 problem. |
| Vitest item + app | `vitest run --project web apps/web/src/features/item apps/web/src/app` | 15 files, 182 tests passed |
| Vitest whole suite + coverage | `pnpm test` (final run) | 52 files, 873 tests passed; Statements 100 % (954/954), Branches 100 % (552/552), Functions 100 % (328/328), Lines 100 % (922/922); thresholds met |
| Coverage of the item feature | per-file `coverage-summary.json` (21 files under `features/item`) | every file 100 / 100 / 100 / 100 (statements / branches / functions / lines) |
| Build | `pnpm -C web build` | exit 0, 288 modules, output in `internal/adapters/http/webui/dist` |
| E2E, run 1 | `pnpm -C web e2e` | 10 passed (9.8 s) |
| E2E, run 2 | `pnpm -C web e2e` | 10 passed (12.0 s) |
| Hygiene | grep in `features/item` for `TODO`, `FIXME`, `any`, `eslint-disable`, `@ts-` | no hits (the only match is the word "any" in a doc comment) |

Whole-suite note: during the middle of the session `LibraryPage.test.tsx` (M2-22, "disables Load more while the next page loads" and "does not move focus again when a refresh adds cards later") failed in two runs, with a different test each time. The final run was fully green. Both tests are M2-22 and not related to the item feature. The existing E2E specs (shell, accessibility) contain no library or item flows, so there was no E2E failure to attribute and no `playwright-report` needed saving.

## Real-browser check

Setup: the server was built with `go build -tags webui -o <scratchpad>\bin\mivo.exe ./cmd/mivo` (identical to `go run -tags webui ./cmd/mivo`, but easier to stop). It ran with isolated `MIVO_*` and OS directories on 127.0.0.1:18423 and was stopped afterwards. The driver script was `<scratchpad>\item-browser-check.cjs` (Chromium via Playwright 1.64.1, `@axe-core/playwright`). The result was 95 of 96 checks passing; the one failure is attributed below.

The live item API is not a pure 501 stub any more. A malformed id returns 400 and a well-formed one (UUID) returns 501.

Part A – live server, no mocks:
- `/items/<uuid>` (501): after the one automatic retry the page shows the error heading "Item could not be loaded" and the server-failure message in a `role=alert`. There is exactly one h1 and a "Try again" button. The German variant shows "Eintrag konnte nicht geladen werden" and "Erneut versuchen".
- Keyboard: "Try again" is reachable by Tab (5 presses) and shows a focus ring (solid 2 px). Enter shows the `role=status` "Loading item details", and after the repeated error focus is on the h1 (`h1:Item could not be loaded`). The next Tab reaches "Try again" again. Space also activates it and ends with focus on the h1.
- `/items/abc` (400): shows "Item not found" without a retry button. "Back to home" is reachable by Tab and Enter navigates to `/`.
- Aborted request (connection refused): shows "The server could not be reached. Check that it is running, then try again."
- axe on the error state (en and de), not-found (live 400) and the network-error state: 0 violations at any impact level.

Part B – real built app and server, only `GET /api/v1/items/*` fulfilled from fixtures (the plan's HDR movie, probe-failed, probe-timed-out, unknown error code, no files, long title):
- Chips list (`aria-label` "Technical details"): `["MKV","HEVC 10-bit HDR10","AC-3 5.1","SRT"]`, which joins to `MKV · HEVC 10-bit HDR10 · AC-3 5.1 · SRT`. The German UI keeps the technical labels and translates only the list label ("Technische Details").
- Facts: "1 h 30 min · 4K" (de: "1 Std. 30 Min. · 4K"). File: `Big.Buck.Bunny.mkv`, "276.1 MB" (de: "276,1 MB", "Dateiname", "Größe").
- Audio tracks (en): "German · AC-3 5.1 · Default" and "English · Director's commentary · AAC 2.0". de: "Deutsch … Standard", "Englisch".
- Subtitle tracks (en): "English · SRT · Forced" and "Unknown language · SRT". de: "Englisch … Erzwungen" and "Unbekannte Sprache". The hint "Subtitles are listed here, but the player does not show them yet." is visible (de: "Untertitel werden hier aufgelistet, …").
- Play link with the awkward id `season 1/ep#1?` and fileId `file/1 x&y`: the API request path is `season%201%2Fep%231%3F`, the href is `/items/season%201%2Fep%231%3F/play?fileId=file%2F1+x%26y`, and Enter on the focused link navigates there. Decoded, the route params round-trip exactly. The router writes a space in the query as `+`, which is valid.
- Disabled Play (probe_failed, probe_timed_out, unknown error code, no file): it is a `<button aria-disabled="true">`, with no `disabled` attribute and no link. It is reachable by Tab (focusable). Its `aria-describedby` resolves to the visible explanation, for example "This file cannot be played because the server could not read it." (de: "Diese Datei kann nicht abgespielt werden, weil der Server sie nicht lesen konnte."). Enter, Space and a forced click leave the URL unchanged. A failed probe shows no chips list and both track lists show their empty text.
- Loading: the `role=status` "Loading item details" appears and an sr-only h1 "Item details" is present. The state is replaced by the item when the response arrives.
- 404: "Item not found" with the "Back to home" link to `/`.
- Retry that succeeds (500, 500, then 200): 2 requests before the error (initial plus one automatic retry), the manual retry makes request 3, and focus lands on the item title h1.
- axe (`serious`/`critical` and all lower impacts): 0 violations on the loaded item (en and de), unavailable Play (en and de), the loading state, not-found (404) and the item at 320 px with the sidebar collapsed.
- Console: the only entries are the browser's own "Failed to load resource" lines for the expected 400, 501 and refused requests; no page errors and no React warnings.
- Screenshots are in the scratchpad `shots` folder.

Failed check, not attributable to M2-23: at a 320 px viewport with the sidebar expanded (the default), `document.documentElement.scrollWidth` is 346–349 against 320, and the item h1 is 0 px wide because the 240 px sidebar leaves no room. The home page at the same width overflows the same way (349 against 320), so this is the app shell (`AppLayout`/`Sidebar`, not responsive below roughly 480 px). With the sidebar collapsed, the item page has no overflow with a 70-character title and a 124-character file name at 320 px (320 against 320), and at 480 px (480 against 480) and 768 px expanded (768 against 768). Reviewer note for the shell owner, not a blocker for M2-23.

## Acceptance criteria

1. `/items/$itemId` loads `GET /api/v1/items/{itemId}`; title, duration, file name and size, chips from the `@mivo/player-core` label functions by a pure function with a table test: **met**. Evidence: Part B chips, facts and file output above. `lib/technicalChips.ts` and `lib/formatLabels.ts` call `containerLabel`, `codecLabel`, `channelLabel`, `hdrLabel` and `bitDepthLabel` from player-core; `lib/technicalChips.test.ts` and `lib/formatLabels.test.ts` are table tests (corpus codecs, HDR10 and HLG, missing fields); all tests pass.
2. Track lists with language via `Intl.DisplayNames` (fallback to the code), title, default/forced flags: **met**. Evidence: Part B track checks in en and de; `lib/languageName.ts` and its test (cases for unknown and invalid codes).
3. Large Play button linking to `/items/$itemId/play?fileId=<primary file>`, disabled with an explanation on a probe error: **met**. Evidence: Part B Play-link and disabled-Play checks (aria-disabled, focusable, announced reason, encoded ids).
4. Unknown item gives a not-found message with a link back; network error offers retry: **met**. Evidence: A3/A5/A6 and B5/B6 above.
5. Strings only in `features/item/locales/{en,de}.json`: **met**. The components contain no user-visible literals (read all five); `ItemPage.test` renders in de.
6. Tests with Vitest and Testing Library plus a fake API local to the feature (render, chips, disabled Play, not found), coverage thresholds hold: **met**. Evidence: `features/item/test/fakeItemServer.ts` and `renderItemPage.tsx`; `src/test/**` is not touched; item feature at 100 % in every metric and the whole suite at 100 % against the 70 % threshold.
7. Gates `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`: **met**. Evidence: Gates table above; all exit 0 in the final state.
8. Notes: no artwork yet (neutral poster placeholder with the title initial): met. Muted subtitle hint: met. Do not touch `src/test/**` or `pnpm-lock.yaml`: met (no item-related change in either).
9. Round-2 accessibility fixes (`useHeadingFocusAfterRetry`, `useDeferredAnnouncement`): **verified in a real browser** (focus lands on the h1 after a retry that fails again and after one that succeeds; the status region announces loading; 0 axe violations in every state).

## Findings

- None blocking for M2-23.
- Informational: the app shell overflows horizontally at 320 px with the sidebar expanded (see above). Out of scope for this task.
- Informational: the h1 receives programmatic focus after a retry but shows no focus ring (`focus-visible:outline-none`). That is intentional for a non-interactive heading and screen readers announce it.
- Informational: M2-22's `LibraryPage.test.tsx` was intermittently red mid-session (two different tests in two runs; green in the final run). M2-22's own report should cover its stability.

## History

- Round 2: all gates re-run after the reviewer's a11y minors were fixed (`useHeadingFocusAfterRetry`, `useDeferredAnnouncement`); real-browser and axe checks added. PASS.
- Round 1: not recorded in this file (no earlier M2-23 report existed).
