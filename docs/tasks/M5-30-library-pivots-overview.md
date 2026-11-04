# M5-30 Library pivots and Overview rows
Owner: mivo-frontend
Depends on: M5-13, M5-24; prototype `browse.html` approved
Files/areas: `web/apps/web/src/features/library/**` (page split: `LibraryPage` -> `OverviewPivot`, `BrowsePivot`), router `validateSearch` (`view`), locales

## Goal
Every library page has tabs Overview and Browse; Overview shows the server rows of the library
(LB-02...04, LB-09); the last used pivot is remembered per library and device; a new library
opens on Browse.

## Acceptance criteria
- [ ] `?view=overview|browse` validated in `router.ts`; invalid values fall back to the
      remembered pivot; tab change updates the URL without a full reload.
- [ ] Overview renders `/libraries/{id}/rows` with `MediaRow`; too little data shows the hint
      from the contract instead of an empty page.
- [ ] Remembered pivot per library in local storage; a library without remembered pivot opens
      Browse.
- [ ] Existing grid behaviour (Load more) is untouched until M5-34.
- [ ] Tests: router validation, pivot memory function, hint state, axe clean; existing
      `LibraryPage` tests updated, not deleted.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Keep `LibraryPage` < 150 lines by moving state to hooks. M7 rows (continue watching, not
watched yet) arrive as new row kinds only.
