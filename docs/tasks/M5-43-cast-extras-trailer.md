# M5-43 Cast and crew, extras, trailer
Owner: mivo-frontend
Depends on: M5-40, M5-24; M3 NFO people and extras (M3-10b, M3-07b)
Files/areas: `web/apps/web/src/features/item/**` (`components/CastRow.tsx`, `ExtrasRow.tsx`, `hooks/useExtras.ts`), locales

## Goal
Detail pages show a cast and crew row (names and roles from local NFO, round portraits only
when the API provides an image) and an extras row (trailers, featurettes, behind the scenes,
deleted scenes) with a trailer button in the action bar (DT-08, DT-09, DT-05 trailer part).

## Acceptance criteria
- [ ] Cast row: initials placeholder when no portrait; no link to a person page yet (M6);
      rows hidden when empty.
- [ ] Extras row groups by extra kind with 16:9 cards; Play starts the extra through the
      normal player route.
- [ ] Trailer button plays the first trailer extra; hidden when none.
- [ ] Uses `MediaRow`; keyboard and axe as there.
- [ ] Tests: hooks with fake API, grouping lib, empty states.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Check the current item/children contract for people and extras; if the item response lacks
`people`, report it for an M3/M5 backend fix instead of guessing.
