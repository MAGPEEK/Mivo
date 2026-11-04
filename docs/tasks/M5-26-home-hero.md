# M5-26 Home hero
Owner: mivo-frontend
Depends on: M5-25
Files/areas: `web/apps/web/src/features/home/components/Hero*.tsx`, `features/home/hooks/useHeroRotation.ts`, device-preference key (read by M5-80), locales

## Goal
Optional hero above the rows: backdrop, title, summary line, Play and "More info", rotating
through up to 5 titles every 8 s (HO-01).

## Acceptance criteria
- [ ] Candidates: server hero list (recently added with backdrop artwork); no artwork -> no hero.
- [ ] Rotation pauses on hover/focus and when the tab is hidden; disabled with
      `prefers-reduced-motion` (static first item with manual dots).
- [ ] Dots are buttons with names; the hero is a labelled region; text contrast >= AA on the
      scrim (unit test of the scrim token choice, axe in component test).
- [ ] Device setting `hero.enabled` (default on) hides it; Play starts the item, More info
      opens the detail page.
- [ ] Tests: rotation hook with fake timers, preference off, no-artwork case.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Needs the hero candidate list in the rows contract (add `hero` to M5-12 if absent: flag to the
orchestrator). Logos and TMDB backdrops arrive with M6; show the title as text now.
