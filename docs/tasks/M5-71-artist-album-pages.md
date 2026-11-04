# M5-71 Artist and album pages
Owner: mivo-frontend
Depends on: M5-40, M3-13h; prototype `media.html` approved
Files/areas: `web/apps/web/src/features/music/components/{ArtistPage,AlbumPage,TrackList}.tsx`, router entries, locales

## Goal
Artist page (albums as a row/grid, top tracks when data exists) and album page (cover, year,
track list with disc/track numbers and durations, Play/Shuffle/Add to queue) in the common
detail layout.

## Acceptance criteria
- [ ] Album page: multi-disc albums grouped by disc; tracks without number last; click on a
      track plays the album from there; Play and Shuffle fill the queue.
- [ ] Artist page: albums sorted by year; singles/compilations not invented (only what the API
      delivers); missing artwork placeholders (1:1).
- [ ] Items of type track/album appear with correct cards in the Browse grid and search.
- [ ] Tests: track grouping lib, pages with fake API, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
M3-13h delivered basic music listing; extend, do not duplicate its components. Bio and similar
artists are M6.
