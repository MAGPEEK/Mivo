# M5-72 Photo timeline and lightbox
Owner: mivo-frontend
Depends on: M5-34, M3-15i; prototype `media.html` approved
Files/areas: `web/apps/web/src/features/photos/components/{PhotoTimeline,Lightbox,ExifPanel}.tsx`, `lib/groupByDate.ts`, `hooks/useLightbox.ts`, locales

## Goal
Photos library shows a virtualised timeline grouped by month/day (EXIF date, fallback file
date) and a lightbox with next/previous, zoom, EXIF panel and slideshow.

## Acceptance criteria
- [ ] Timeline uses the virtual grid hook with date headers; group boundaries are correct
      across time zones (table tests with fixed dates).
- [ ] Lightbox: arrow keys and swipe navigate, `+`/`-`/wheel zoom and pan, Escape closes and
      restores focus to the thumbnail; adjacent images preloaded; URL keeps the open photo.
- [ ] EXIF panel shows camera, lens, exposure, date, GPS only as text (no map, no external
      requests); fields absent are omitted.
- [ ] Slideshow with interval, pauses on interaction, respects reduced motion.
- [ ] Tests: groupByDate, lightbox keyboard/URL, EXIF formatting, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Originals only in the lightbox at fit size via the image endpoint's allowed sizes; RAW/HEIC per
M3-15 capability decisions.
