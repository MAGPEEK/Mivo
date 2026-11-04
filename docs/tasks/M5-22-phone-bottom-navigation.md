# M5-22 Phone layout: bottom navigation
Owner: mivo-frontend
Depends on: M5-20; prototype `browse.html` approved
Files/areas: `web/apps/web/src/app/layout/**`, `shared/ui/` (BottomNav), breakpoints in `ui-tokens` if missing, locales

## Goal
Below the small breakpoint the sidebar is replaced by a bottom navigation: Home, Libraries
(sheet with the pinned list), Search, Settings (SH-17); content gets safe-area padding.

## Acceptance criteria
- [ ] At widths < small breakpoint: bottom nav visible, sidebar hidden; at larger widths unchanged.
- [ ] Libraries opens a `Sheet` with the pinned libraries and a link to "All libraries".
- [ ] Search tab focuses/open the search UI (M5-50 hooks in later; until then the existing field).
- [ ] Touch targets >= 44 px; current route marked `aria-current`.
- [ ] Tests: component tests with a faked `matchMedia`; Playwright viewport check is part of M5-91.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
The mini player and music bar sit above the bottom nav (M5-67, M5-70): expose the nav height as
a CSS variable.
