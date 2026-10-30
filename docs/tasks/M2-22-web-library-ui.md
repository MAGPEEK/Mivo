# M2-22 Web: libraries in the sidebar, library grid and scan button
Owner: mivo-frontend
Depends on: M2-01, M2-07, M2-14 (labels)
Files/areas: `web/apps/web/src/features/library/**`, `web/apps/web/src/app/layout/AppLayout.tsx`,
`web/apps/web/src/shared/ui/{Sidebar.tsx,Card*.tsx,Badge*.tsx}` (+ tests), `web/apps/web/src/test/**`
(shared test helpers; M2-23 must not edit them in the same wave)

## Goal
The user sees the configured libraries in the sidebar, opens one, browses its videos as a poster
grid with technical chips, and starts a scan with live status (ADR 0011 design, ADR 0022).

## Acceptance criteria
- [ ] Sidebar shows a "Libraries" section with one link per library (icon by kind, unavailable
      libraries marked); data via a TanStack Query hook in `features/library`, passed into the
      shared Sidebar as props.
- [ ] `/libraries/$libraryId`: heading, item count, grid of cards (2:3 placeholder poster with the
      title initial, title, duration, one tech chip such as `MKV · H.264 · AAC`; unavailable items
      greyed with a label); "Load more" pages through `nextCursor` (`useInfiniteQuery`).
- [ ] Scan button: `POST …/scan`, then polls the library status every 2 s while `running`
      (named constant), shows counts when finished and the translated error for `failed`
      (`root_unavailable` and generic). The button is disabled while running.
- [ ] Empty states: no libraries configured (text explains `libraries` in `config.yaml` /
      `MIVO_LIBRARIES` per OD-1, product name from brand), empty library ("Run a scan").
- [ ] Clicking a card navigates to `/items/$itemId`.
- [ ] Strings in `features/library/locales/{en,de}.json` only; German complete.
- [ ] Accessibility: grid as a list with labelled links, scan button with `aria-busy`, status
      text in a polite live region; axe has no serious/critical findings in a component test.
- [ ] Tests (Vitest + Testing Library + fake API): sidebar links, grid paging, scan polling with
      fake timers, empty states, error state. Coverage thresholds hold.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`, `pnpm -C web e2e`
      (existing specs still green).

## Notes
- No virtualisation yet (TanStack Virtual comes with milestone 5); the page size keeps the DOM
  small.
- Chip text uses the label functions of `@mivo/player-core` (M2-14); the library feature only
  chooses which labels to join.
- Colours only from ui-tokens; no hex literals (ESLint rule).

## Orchestrator note (from M2-02)
- The domain stores two probe error codes, `probe_failed` and `probe_timed_out`. Both need en/de
  strings; unknown codes fall back to a generic text.

## Orchestrator note (from M2-07)
- Feature translation namespaces are registered in `web/apps/web/src/app/i18n.ts` (one line per
  feature), not in `shared/i18n/resources.ts`; the i18next type augmentation lives in
  `src/app/i18next.d.ts`. Features only edit their own `locales/{en,de}.json`.
- Pages receive route params as props (`LibraryPage {libraryId}`, `ItemPage {itemId}`,
  `PlayerPage {itemId, fileId, startAt}`); `features/player/lib/playerSearch.ts`
  (`validatePlayerSearch`) already exists.
- (from M2-14) `apps/web` depends on `@mivo/player-core` (workspace) since M2-14; use its
  `containerLabel`, `codecLabel`, `channelLabel`, `hdrLabel`, `bitDepthLabel`,
  `resolutionLabel(height, width?)` for chips. Do not change pnpm-lock.yaml in wave 3 (M2-27 owns
  it in wave 4).

## Orchestrator note (round 2)
- The scan button and "Load more" use `aria-disabled` (plus `aria-busy`) instead of native
  `disabled` while busy: native `disabled` dropped keyboard focus to body. Clicks are ignored while
  busy and the state is announced; this satisfies the intent of "disabled while running".
