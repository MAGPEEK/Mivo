# M5-42 Media info dialog
Owner: mivo-frontend
Depends on: M5-20; prototype `detail.html` approved
Files/areas: `web/apps/web/src/features/item/components/MediaInfoDialog.tsx`, `lib/mediaInfoSections.ts`, `lib/mediaInfoJson.ts`, public export for the player feature, locales

## Goal
A dialog shows library and library-relative path, media block, file block and one block per
video/audio/subtitle stream, with "Copy as JSON" (PL-36, DT-11); also opened from the player.

## Acceptance criteria
- [ ] Blocks and fields as listed in ux-spec 9.6; fields the API does not deliver are omitted
      (no empty rows); no absolute path anywhere.
- [ ] Items with several files list each file with size and container (ED-06 read-only).
- [ ] "Copy as JSON" copies a stable JSON of the displayed data and announces success;
      clipboard failure shows an error.
- [ ] Focus trap, Escape, returns focus to the opener.
- [ ] Tests: section builder table tests (movie, multi-audio, external subtitle), JSON copy
      with faked clipboard, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Reuses `formatLabels`, `fileSize`, `duration`, `languageName` helpers. If the item API lacks
fields the spec wants (profile, level, chroma), list them in the REPORT for a backend follow-up.
