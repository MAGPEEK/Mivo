# M5-21 Sidebar: pin, reorder, all libraries, reset
Owner: mivo-frontend
Depends on: M5-20; prototype `browse.html` approved
Files/areas: `web/apps/web/src/app/layout/**`, `shared/ui/Sidebar.tsx`, new `features/sidebar-layout` hook (local storage), locales

## Goal
The sidebar shows pinned libraries in the user's order; "All libraries" shows the full list with
pin toggles; the layout can be reset (SH-12, SH-13, SH-14, SH-16).

## Acceptance criteria
- [ ] Pins and order persist per device (local storage, versioned key); new libraries appear
      pinned at the end; removed libraries drop out silently.
- [ ] Reorder by drag and drop and by move up/down buttons (keyboard); live-region announces moves.
- [ ] "Reset layout" restores server order, all pinned; button also exposed for M5-80.
- [ ] Corrupt/legacy local storage falls back to defaults (test).
- [ ] Tests: pure ordering/merge functions table-tested in `lib/`; component tests for both
      reorder paths; axe clean. Existing sidebar tests stay green.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Storage sits behind a small `LayoutStore` interface so M7 can swap it for the server
(per profile, with one-time import).
