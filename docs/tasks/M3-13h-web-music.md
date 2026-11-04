# M3-13h Web: album grid, album page with tracks, audio playback, E2E
Owner: mivo-frontend
Depends on: M3-13b, M3-13g
Files/areas: `web/apps/web/src/features/music/**`, router entry, `e2e/music.spec.ts`

## Goal
A music library shows square album covers; an album page lists tracks by disc and plays one.

## Acceptance criteria
- [ ] Library grid renders albums with `square` covers and artist name; empty and error states as in the video grid.
- [ ] Album page: header cover, tracks grouped by disc, play starts audio through `player-core`.
- [ ] Add-library dialog shows the music tile (from `libraryKinds`).
- [ ] Tests: Vitest for hooks/components; Playwright: add music library from the corpus, open album, play first track (time advances).
- [ ] Gates: web lint, test, build, e2e pass.

## Notes
Artist pages and the full music player are M5; no new layout system.
