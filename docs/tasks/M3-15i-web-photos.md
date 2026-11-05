# M3-15i Web: album grid, photo thumbnails, E2E
Owner: mivo-frontend
Depends on: M3-15b, M3-15h
Files/areas: `web/apps/web/src/features/photos/**`, router entry, `e2e/photos.spec.ts`

## Goal
A photo library shows albums and a thumbnail grid; opening a photo shows it large with basic EXIF text.

## Acceptance criteria
- [x] Album grid and thumbnail grid with lazy loading and an "unsupported" placeholder.
- [x] Simple photo view (large image, date, camera); lightbox, timeline and map are M5.
- [x] Tests: Vitest for components/hooks; Playwright: open a photo album, thumbnails load (images decoded), open one photo.
- [x] Gates: web lint, test, build, e2e pass.

## Notes
Grid virtualisation is M5; keep the page size bounded by the API cursor.
