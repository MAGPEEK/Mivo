# M5-24 MediaRow component
Owner: mivo-frontend
Depends on: M5-20; prototype `browse.html` approved
Files/areas: `web/apps/web/src/shared/ui/MediaRow.tsx` (+ `lib/rowScroll.ts`), `features/library/components/ItemCard.tsx` (card formats only), locales

## Goal
One horizontally scrolling row with title, "See all" link and arrow buttons, used by Home and
the library Overview (HO-02); card formats poster 2:3, episode 16:9, square 1:1.

## Acceptance criteria
- [ ] Prev/next buttons appear on hover and focus and disable at the ends; left/right arrow
      keys move focus between cards (roving tabindex); Home/End jump.
- [ ] Rows with many items render only visible cards (horizontal virtualisation); a row with
      5 items renders all.
- [ ] Card slot for a badge/progress overlay exists (filled by M7) and the episode label
      `S3 . E9` comes from the shared formatter.
- [ ] Tests: scroll maths in `lib/` table-tested; component tests with faked layout; axe clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Use TanStack Virtual (also used in M5-34) with `horizontal: true`; respect reduced motion for
smooth scrolling.
