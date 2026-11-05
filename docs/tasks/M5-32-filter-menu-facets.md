# M5-32 Filter menu with facets and chips
Owner: mivo-frontend
Depends on: M5-31
Files/areas: `web/apps/web/src/features/library/**` (`components/FilterMenu.tsx`, `FilterChips.tsx`, `lib/filterRegistry.ts`, `hooks/useFacets.ts`), locales

## Goal
Quick filters (single choice) and field filters (combinable, removable chips) with value counts
from the server facets (LB-20).

## Acceptance criteria
- [x] Quick filters: All, HDR, Dolby Vision, HDR10+, Dolby Atmos, 4K, Unavailable, Duplicates.
- [x] Field filters: genre, year, decade, content rating, resolution, video codec, audio codec,
      container, audio/subtitle language; each value shows its count; zero-count values
      are hidden or disabled.
- [x] Chips under the toolbar show active filters and remove on click/Delete; "Clear all".
- [x] A filter registry (data, not code branches) lets M6/M7 add fields (people, collection,
      unmatched, watch state) without touching the menu component.
- [x] Facets refetch when other filters change; stale responses are ignored (test).
- [x] Tests: registry/URL mapping table tests, menu keyboard test, chips, axe clean.
- [x] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Language names via the existing `languageName` helper.
