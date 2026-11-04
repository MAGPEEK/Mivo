# M5-40 Detail layout: backdrop, poster, title block, summary
Owner: mivo-frontend
Depends on: M5-20; prototype `detail.html` approved
Files/areas: `web/apps/web/src/features/item/**` (`components/DetailLayout.tsx`, `Backdrop.tsx`, `TitleBlock.tsx`, `Summary.tsx`, `lib/runtimeLabel.ts`), locales

## Goal
All detail pages share one layout: blurred, darkened backdrop (else poster), poster column with
content-rating badge, title block, clamped summary (DT-01, DT-02, DT-03, DT-06).

## Acceptance criteria
- [ ] Backdrop falls back to poster, then plain background; text contrast >= AA over any
      artwork (scrim token; test checks the computed class/token, axe in component test).
- [ ] Title block: title, original title when different, content rating, year, runtime
      ("1 h 39 min" localised en/de), first three genres with "+n" opening all; each genre links
      to Browse filtered by genre (`filter` URL of M5-32); director line when present.
- [ ] Summary clamped to 4 lines with "More"/"Less" (`aria-expanded`); tagline above.
- [ ] Movie, show, season, episode, extra pages use the layout; episodes use a 16:9 still.
- [ ] Missing fields hide their line, never show "undefined"/empty separators (tests).
- [ ] Tests: `runtimeLabel` table tests, layout fallbacks, axe; M3-12/12b page tests stay green.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Image sizes from the allowed set (LB-44); lazy-load the backdrop. Version/audio/subtitle
selectors (DT-07) are M4 and not drawn here; keep the technical chips and track list as today.
