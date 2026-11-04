# M5-25 Home rows
Owner: mivo-frontend
Depends on: M5-13, M5-24; prototype `browse.html` approved
Files/areas: `web/apps/web/src/features/home/**`, `api-client` usage, locales

## Goal
Home renders the rows delivered by `/api/v1/home/rows` (recently added per library with show/
season grouping and "new episodes" badge); "See all" opens the Browse pivot with the row's
filter and sort in the URL (HO-02, HO-05, HO-07).

## Acceptance criteria
- [ ] One `MediaRow` per server row, in server order; titles built from key + params (en/de).
- [ ] Show card with `newEpisodeCount` badge; season card variant; episode label formatting.
- [ ] "See all" navigates to `/libraries/$id?view=browse&sort=...&...` taken from `seeAllQuery`.
- [ ] Zero libraries keeps the existing "No libraries yet" empty state (HO-09); libraries
      without rows show a short hint; loading skeletons; error with retry.
- [ ] Tests: hook with fake API, row-to-route mapping in `lib/`, empty/error states; axe clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Do not compute row content in the client. Unknown row kinds (from a newer server) render as
generic rows or are skipped, never crash.
