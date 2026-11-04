# M5-35 Card overlay, overflow menu, selection mode
Owner: mivo-frontend
Depends on: M5-34, M5-41
Files/areas: `web/apps/web/src/features/library/**` (`components/ItemCard.tsx`, `CardOverlay.tsx`, `SelectionBar.tsx`, `hooks/useSelection.ts`, `lib/selectionRange.ts`), locales

## Goal
Hover/focus overlay with play button and overflow menu on cards, and a multi-select mode with
Shift-range for bulk "Re-analyze" (LB-26, LB-41, LB-43); Play all / Shuffle for the filtered
list (LB-24).

## Acceptance criteria
- [ ] Overlay appears on hover and focus-within, play starts the item, menu uses the item
      actions list of M5-41 (subset for cards).
- [ ] Selection: checkbox per card, Shift-click/Shift+arrows range, Escape clears; selection
      bar shows the count and actions (Re-analyze for admins; others arrive M6/M7).
- [ ] Play all / Shuffle put the current filtered, sorted list into the queue (needs M5-60/65
      API; until then buttons are hidden behind the queue feature check).
- [ ] Range selection works across virtualised, not-yet-loaded ranges using offsets (test).
- [ ] Tests: `selectionRange` table tests, hook tests, overlay component tests, axe clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Watched toggle and edit pencil slots exist but stay empty (M6/M7). Bulk re-analyze calls
M5-17 once per item with a small concurrency limit.
