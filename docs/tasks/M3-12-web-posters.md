# M3-12 Web: posters in the library grid and on the detail page
Owner: mivo-frontend
Depends on: M3-02
Files/areas: `web/apps/web/src/shared/ui/{Poster.tsx,Poster.test.tsx}` (new),
`web/apps/web/src/features/library/**` (`ItemCard.tsx`, `LibraryItemGrid.tsx`,
`lib/itemCard.ts`, hooks, locales, tests), `web/apps/web/src/features/item/components/
ItemDetails.tsx` (poster slot only) and its test, `web/apps/web/src/test/libraryFixtures.ts`,
`web/apps/web/src/test/fakeApi.ts`, `docs/tasks/BACKLOG.md` (only the `web library` scan-button
row)

## Goal
Library grids show real posters (2:3) loaded in the size the card needs from the image
endpoint, with a neutral placeholder while loading or when an item has none; shows appear as
one card each; the movie detail page shows its poster. UX: **LB-40** (M3 part: local artwork),
LB-44 (client side); ADR 0027 §8.

## Acceptance criteria
- [ ] `Poster` component (shared UI, no business logic): props `imageRef?`, `aspect`
      (`poster` 2:3, `still` 16:9), `alt`, `sizes`; renders `<img>` with `src` and `srcset`
      from the api-client helpers (`imageUrl`, `imageSrcSet` with the widths 160–960),
      `loading="lazy"`, `decoding="async"`, fixed aspect box (no layout shift), and a neutral
      placeholder (token colours, kind icon) when there is no image or it fails to load.
- [ ] Library grid cards use `Poster` with the item's `poster`; cards show title and year;
      show cards show "N seasons" (`childCount`) instead of media chips; movie cards keep
      the M2 chips (resolution, unavailable).
- [ ] The item page shows the movie's poster (`artwork.poster`) next to the existing details;
      everything else on the page is unchanged (M3-12b adds the show pages).
- [ ] i18n keys added in `en` and `de` (no hard-coded strings); alt text = item title.
- [ ] BACKLOG `web library` row done and deleted (synchronous ref guard in
      `useLibraryScan.start` against a double POST).
- [ ] Vitest: `Poster` (src/srcset/sizes, placeholder without ref, placeholder after `error`
      event, aspect classes), card rendering for movie and show, item page with and without
      poster; axe check in the existing component tests stays clean.
- [ ] Gates: `pnpm -C web lint` (ESLint, dependency-cruiser, Prettier, tsc), `pnpm -C web
      test` (coverage thresholds), `pnpm -C web build`.

## Notes
- The web app never builds image URLs by hand; only the api-client helpers do (M3-02).
- `sizes` should follow the grid's column widths; until the size slider (LB-28, M5) exists, a
  single `sizes` expression for the current breakpoints is enough.
- No virtualisation or layout overhaul here (M5); keep the M2 grid and its "Load more".
