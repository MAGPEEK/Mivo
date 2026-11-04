# M3-14h Web: book grid, book detail with chapters, E2E
Owner: mivo-frontend
Depends on: M3-14b, M3-14g, M3-13h
Files/areas: `web/apps/web/src/features/audiobooks/**`, router entry, `e2e/audiobooks.spec.ts`

## Goal
Books show as covers with author; the detail page lists chapters and plays from a chapter.

## Acceptance criteria
- [ ] Grid with author line; detail with narrator, chapter list (title, start time).
- [ ] Clicking a chapter starts playback at its offset (multi-file books map to file + local offset).
- [ ] Tests: Vitest for the offset mapping in `player-core`/hook; Playwright: open the M4B book, chapter list shows 3 entries.
- [ ] Gates: web lint, test, build, e2e pass.

## Notes
Playback speed and sleep timer are M5.
