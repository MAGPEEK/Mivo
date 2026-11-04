# M5-50 Search dropdown
Owner: mivo-frontend
Depends on: M5-15, M5-20; prototype `browse.html` approved
Files/areas: `web/apps/web/src/features/search/**` (new: `components/SearchBox.tsx`, `SearchDropdown.tsx`, `hooks/useSearchSuggestions.ts`, `lib/recentSearches.ts`), `app/layout` wiring (replaces the plain field, keeps `useSearchShortcut`), locales

## Goal
The top-bar search shows a dropdown after 2 characters (debounced 200 ms) with up to 8 mixed
results (thumbnail, title, year, type, library), a play button per result, and recent searches
when empty (SR-01).

## Acceptance criteria
- [ ] Ctrl+K / Cmd+K focuses the field (existing behaviour kept); Escape closes; arrow keys move
      through results (combobox pattern: `aria-activedescendant`, `aria-expanded`).
- [ ] Enter on a result opens it; Enter in the field navigates to `/search?q=`.
- [ ] Debounce 200 ms; stale responses ignored; < 2 chars triggers no request (fake timers).
- [ ] Recent searches per device (max 8, dedup, clear action); never stored when private
      query is empty.
- [ ] Result overflow menu uses `itemActions` surface "search".
- [ ] Tests: lib tests, hook with fake API/timers, combobox keyboard, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
On phones the Search tab of the bottom nav (M5-22) opens a full-screen variant of the same box.
