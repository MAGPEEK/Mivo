# M5-20 Shared UI primitives: menu, sheet, tabs, segmented control
Owner: mivo-frontend
Depends on: none
Files/areas: `web/apps/web/src/shared/ui/**` (new `Menu`, `Sheet`, `Tabs`, `SegmentedControl`, `Dialog` base reusing `useModalFocus`), `index.ts`, locales

## Goal
The accessible building blocks later tasks share (overflow menus, phone bottom sheets, pivots,
view toggles), so feature folders do not each invent them.

## Acceptance criteria
- [ ] `Menu` (button + popup): arrow-key navigation, Escape closes and returns focus,
      type-ahead, `role=menu/menuitem(radio|checkbox)`; opens upward/downward by space.
- [ ] `Sheet`: modal bottom sheet on narrow screens, dialog elsewhere; focus trap, Escape,
      scroll lock, `prefers-reduced-motion` respected.
- [ ] `Tabs` with roving tabindex and `aria-selected`; `SegmentedControl` as radio group.
- [ ] Tests: Vitest + Testing Library per primitive incl. keyboard paths; axe clean.
- [ ] No feature imports; no business logic; dependency-cruiser passes.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
No new dependency unless a11y-correct and tiny; prefer small own components over a UI kit.
Colours come from `ui-tokens`.
