# M2-07 Web: routes and per-feature translations for library, item and player
Owner: mivo-frontend
Depends on: none
Files/areas: `web/apps/web/src/app/router.ts`, `web/apps/web/src/app/layout/**` (only to move the
layout into a layout route), `web/apps/web/src/app/App*.tsx`, `web/apps/web/src/shared/i18n/**`,
`web/apps/web/src/features/{library,item,player}/` (new: `index.ts`, placeholder page,
`locales/{en,de}.json`), related tests

## Goal
The router and translation setup are ready for three new features, so the library, item and
player tasks (M2-22, M2-23, M2-27, M2-30) work in their own folders in parallel without editing
shared files.

## Acceptance criteria
- [ ] Routes: `/libraries/$libraryId` and `/items/$itemId` inside the app layout (sidebar, top
      bar); `/items/$itemId/play` **outside** the layout (full-screen player) via a pathless
      layout route. Search params of the player route (`fileId?: string`, `t?: number ≥ 0`) are
      validated by a small typed validator; invalid values are dropped, not thrown.
- [ ] Each new feature exports a placeholder page from its `index.ts`; the pages render a heading
      from their own namespace.
- [ ] Translations are split per feature: `src/features/<feature>/locales/{en,de}.json`, one
      i18next namespace per feature (`library`, `item`, `player`), registered once in
      `shared/i18n/resources.ts`. The existing locale test covers every namespace (German typed
      as the English shape, no extra or empty keys).
- [ ] Not-found handling and the existing shell behaviour (M1-15/M1-17 specs) are unchanged;
      `pnpm -C web e2e` still passes.
- [ ] Tests: router tests for the three routes (memory history), player route renders without
      the sidebar, search-param validation table test, locale parity test.
- [ ] Gates: `pnpm -C web lint` (ESLint, dependency-cruiser, Prettier, tsc), `pnpm -C web test`
      with coverage thresholds, `pnpm -C web build`, `pnpm -C web e2e`.

## Notes
- Feature tasks later only add keys to their own JSON files and replace their placeholder page;
  they never touch `router.ts` or `resources.ts`. Keep the registration generic enough for that.
- `shared` must not import `features` (dependency-cruiser): register namespaces in the app layer
  or keep `resources.ts` free of feature imports by importing the JSON files only in `src/app`.
  Choose one and document it in the file.
- Code-based routing stays (ADR 0005 note in `router.ts`).
