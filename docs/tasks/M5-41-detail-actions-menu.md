# M5-41 Detail actions and item actions menu
Owner: mivo-frontend
Depends on: M5-20, M5-17, M5-42; prototype `detail.html` approved
Files/areas: `web/apps/web/src/features/item/**` (`components/ActionBar.tsx`, `ItemActionsMenu.tsx`, `lib/itemActions.ts`), `shared/ui` consumers only, locales

## Goal
Action buttons on detail pages (Play with split menu "Play from start", overflow menu) and one
reusable item-actions definition used by detail pages, cards, search results and the player's
More menu (DT-05, DT-11, ux-spec section 7).

## Acceptance criteria
- [ ] `itemActions(item, surface)` pure function returns the M5 actions valid for that surface:
      Play, Play from start (only when a position exists, M7: hidden now), Play next, Add to
      queue, Re-analyze (admin), Media info; M6/M7 actions are absent, not disabled.
- [ ] Re-analyze calls M5-17 and shows result/errors in a live region; Media info opens M5-42.
- [ ] Play next / Add to queue are shown only when the queue (M5-60) is available.
- [ ] Trailer button appears when a local trailer extra exists (data from M5-43).
- [ ] Tests: `itemActions` table tests per surface and kind, menu keyboard, error path.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Admin-only actions: until M7 every local-guard user is admin; check a single `isAdmin` hook
so M7 swaps the source.
