# M5-93 E2E: music, photos, audiobooks
Owner: mivo-frontend
Depends on: M5-70, M5-72, M5-73
Files/areas: `web/apps/web/e2e/music-bar.spec.ts`, `photos-lightbox.spec.ts`, `audiobook.spec.ts`

## Goal
Playwright covers the media-type UIs with the music, photo and audiobook corpus (M3-13f, 14f, 15g).

## Acceptance criteria
- [ ] Music: play an album, bar persists across navigation to Home and back, next track works,
      starting a video pauses the bar.
- [ ] Photos: timeline groups, lightbox opens from a thumbnail, arrows navigate, Escape closes
      with focus returned, EXIF panel shows camera text.
- [ ] Audiobook: chapter click seeks, speed applies, resume offer appears after reload.
- [ ] Axe scan on the three pages.
- [ ] Gates: `pnpm -C web e2e` green.

## Notes
Skip with a clear message when a corpus kind is missing (FFmpeg absent), like corpustest.
