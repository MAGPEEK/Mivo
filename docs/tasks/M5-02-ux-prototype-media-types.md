# M5-02 Prototype: music, photos, audiobooks
Owner: mivo-ux
Depends on: none
Files/areas: `docs/design/prototypes/media.html`, `docs/design/prototypes/README.md`

## Goal
Clickable prototype of the screens the ux-spec only references in 5.4 (plan screens 5-7):
artist and album pages with the persistent music bar, photo timeline with lightbox, audiobook
page with chapters and speed.

## Acceptance criteria
- [ ] Music: library artists grid, artist page (albums row, top tracks), album page (track list
      with disc/track numbers), persistent bottom player bar (cover, title, prev/play/next,
      seek, volume, queue, repeat/shuffle) that survives navigation; phone variant above the
      bottom navigation.
- [ ] Photos: month/day grouped timeline, album/folder view, lightbox (zoom, swipe/arrow
      navigation, EXIF side panel, slideshow, close with Escape).
- [ ] Audiobooks: book page with chapter list, resume point, speed 0.5-2.0x, sleep timer
      placeholder (marked later), mini bar behaviour.
- [ ] Interplay rule drawn: only one playback at a time (video mini player vs music bar).
- [ ] Legend with ids, open questions and defaults; README row added, status `draft`; en/de
      toggle, self-contained, brand constant.
- [ ] Gates: none (docs only); owner approval before M5-70...73.

## Notes
Reuse shell, cards and toolbar from `browse.html`; data fields only what M3-13/14/15 deliver.
