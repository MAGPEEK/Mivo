# M5-73 Audiobook UI
Owner: mivo-frontend
Depends on: M5-64, M3-14h; prototype `media.html` approved
Files/areas: `web/apps/web/src/features/audiobooks/**` (`components/BookPage.tsx`, `ChapterList.tsx`, `BookPlayerBar.tsx`, `hooks/useBookPosition.ts`), locales

## Goal
Book page with chapter list, resume point, speed control and a compact player bar that reuses
the music bar mechanics (single playback, queue of one book).

## Acceptance criteria
- [ ] Chapter list from the API with start time and duration; current chapter marked; click
      seeks; previous/next chapter buttons.
- [ ] Speed 0.5-2.0x via the control from M5-64, remembered per book on this device.
- [ ] Resume point stored on this device (local storage keyed by item id) and offered as
      "Continue at 3:12:04"; server-side position arrives with M7 (store behind an interface).
- [ ] Skip back 15 s / forward 30 s; Media Session metadata.
- [ ] Tests: position store, chapter lookup lib (table), page with fake API, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Sleep timer: not in M5 unless the prototype approves it as trivial (flag in REPORT).
