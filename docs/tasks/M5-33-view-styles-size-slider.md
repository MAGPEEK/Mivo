# M5-33 View styles and size slider
Owner: mivo-frontend
Depends on: M5-31
Files/areas: `web/apps/web/src/features/library/**` (`components/ListView.tsx`, `DetailsView.tsx`, `SizeSlider.tsx`, `hooks/useViewPreference.ts`), locales

## Goal
Grid, List (compact rows: title, year, duration, resolution chip) and Details (poster thumbnail,
summary line, technical chips) views plus a 5-step card size slider, remembered per device
(LB-27, LB-28).

## Acceptance criteria
- [ ] View switch is a `SegmentedControl`; choice in URL-independent device preference.
- [ ] Slider has 5 discrete steps, keyboard operable, and asks the image endpoint for a size
      from the allowed set (LB-44), never a free size.
- [ ] List/Details reuse the chip helpers of `item/lib` through a public feature export (no
      deep imports; dependency-cruiser passes).
- [ ] Episode and album cards keep their aspect ratios in every step.
- [ ] Tests: size-to-image-size mapping table test, preference hook, each view renders.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Reused by season pages (M5-44), so views take an `items` prop and know nothing of libraries.
