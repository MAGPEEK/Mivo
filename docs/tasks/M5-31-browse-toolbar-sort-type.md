# M5-31 Browse toolbar: URL state, type switch, sort, count
Owner: mivo-frontend
Depends on: M5-11, M5-20; prototype `browse.html` approved
Files/areas: `web/apps/web/src/features/library/**` (`components/BrowseToolbar.tsx`, `lib/browseQuery.ts`, `hooks/useBrowseState.ts`), router `validateSearch`, locales

## Goal
The Browse pivot has a toolbar (one line desktop, sheet on phones) with type switch (Movies/
Shows/Seasons/Episodes/Folders entry), sort menu with direction toggle, and item count (LB-21,
LB-22, LB-23); all state lives in the URL.

## Acceptance criteria
- [ ] Sort options match the server contract (rating/progress options hidden until M6/M7);
      direction toggle; "random" keeps its seed in the URL so reload is stable.
- [ ] Type options depend on the library kind; counts shown as "367 movies" (plural forms en/de).
- [ ] `browseQuery.ts` parses/serialises URL <-> API query (table tests: unknown values are
      dropped, defaults omitted from the URL).
- [ ] Changing sort/type resets the cursor and scroll position.
- [ ] Phone: same controls in a `Sheet`.
- [ ] Tests: lib table tests, hook with fake API, toolbar keyboard test, axe clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
This task defines the URL contract for M5-32...36 and M5-25 "See all"; keep names identical to
the API query parameters.
