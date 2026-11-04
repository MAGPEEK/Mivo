# M5-34 Virtualised grid and A-Z bar
Owner: mivo-frontend
Depends on: M5-31, M5-33
Files/areas: `web/apps/web/src/features/library/**` (`components/VirtualItemGrid.tsx`, `AlphaJumpBar.tsx`, `hooks/useVirtualItems.ts`, `lib/letterOffsets.ts`), remove "Load more" path, locales

## Goal
The grid renders 50,000+ items smoothly with TanStack Virtual, fetching pages on demand, and a
right-edge `# A-Z` bar jumps to the first title of a letter in title sort (LB-29, LB-30).

## Acceptance criteria
- [ ] Only visible rows (+ overscan) are in the DOM (test counts nodes for 50,000 faked items);
      pages load by cursor as the user scrolls; skeleton cards for unloaded ranges.
- [ ] A-Z bar visible only for title sort; absent letters dimmed and not focusable as
      active; click/Enter scrolls to the offset from `.../letters` (fetching the needed page).
- [ ] Keyboard: arrow keys move focus in the grid, focus survives virtualisation, Page Up/Down.
- [ ] Changing filter/sort resets scroll; scroll position is restored on back navigation.
- [ ] Tests: offset-to-scroll maths table-tested; hook with fake pages; axe clean; existing
      library tests migrated.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Row height depends on the size slider step; recompute columns with a ResizeObserver (faked in
tests). Photo timeline (M5-72) reuses the virtual hook.
