# M5-51 Search results page
Owner: mivo-frontend
Depends on: M5-50, M5-34
Files/areas: `web/apps/web/src/features/search/**` (`components/SearchResultsPage.tsx`, `lib/searchTabs.ts`), route `/search` in `app/router.ts` (`validateSearch`: `q`, `type`), locales

## Goal
`/search?q=&type=` with pivots Top results, Movies, Shows, Episodes, Music (artists, albums,
tracks), Photos, Audiobooks; each pivot is a grid of the card type (SR-02). People (M6) and
Collections (M7) are absent.

## Acceptance criteria
- [ ] Pivot tabs show only types that have hits (counts from the response); URL keeps `q`/`type`.
- [ ] Typed pivots page by cursor with the grid hook; "Top results" groups by type with
      "See all" to the typed pivot.
- [ ] No results shows suggestions text and a link to the libraries; error with retry.
- [ ] Query is trimmed; very long input is clamped; focus moves to the heading on navigation.
- [ ] Tests: router validation, tab derivation lib, page with fake API, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Cards per type: poster, 16:9 for episodes, 1:1 for albums/photos (shared card formats).
