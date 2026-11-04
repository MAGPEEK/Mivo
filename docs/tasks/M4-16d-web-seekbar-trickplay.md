# M4-16d Web: trickplay preview on the seek bar
Owner: mivo-frontend
Depends on: M4-16a, M4-14a
Files/areas: `web/apps/web/src/features/player/**`

## Goal
Hovering or scrubbing the seek bar shows the preview frame and time (PL-05); missing trickplay degrades to time only.

## Acceptance criteria
- [ ] On first play of an item the app asks `GET trickplay` (404 → `POST` once, quietly); previews appear after generation without reload.
- [ ] Tooltip shows a tile cropped via CSS background position from `trickplayTile`; keyboard scrubbing (arrow keys) shows the same preview.
- [ ] No request storm: sheets load lazily, at most 2 in flight, cached by the browser (`max-age`).
- [ ] Tests: component test with a fake manifest and sheet URL; hook test for the request-once behaviour.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- Chapter ticks come later (M4-22).
