# M5-61 Player layout and controls
Owner: mivo-frontend
Depends on: M5-60, M5-20; prototype `detail.html` approved
Files/areas: `web/apps/web/src/features/player/components/{PlayerTopBar,PlayerControls,PlayerPage}.tsx`, new `VolumeControl.tsx`, `TitleBlock`, `hooks/usePlayerQueue.ts`, locales

## Goal
Bring the player to the final layout: top bar with Minimise and Fullscreen, left block (title
link, `S1 . E3 . Title`, year, elapsed/total), centre controls (previous, back 10 s, play/pause,
forward 30 s, next, stop), right controls (mode indicator slot, More, repeat, shuffle, options,
queue, volume) (PL-01...04).

## Acceptance criteria
- [ ] Previous/next disabled without a queue; stop ends the session and returns to the detail
      page; Minimise emits an event consumed by M5-67 (no-op until then).
- [ ] Volume: mute button and slider (keyboard operable), value kept per device.
- [ ] Repeat cycles off/all/one and shuffle toggles, both via the queue from M5-60; state is
      announced (`aria-pressed`, live text).
- [ ] Auto-hide of controls (PL-06) still works; all buttons have accessible names; layout
      holds at 360 px width (phone variant from the prototype).
- [ ] Existing PlayerPage tests (controls, overlay, keyboard) stay green; new tests per control.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
PlayerPage is already large: split into small components, keep each file well under 200 lines.
Slots (props) for mode indicator, More menu, options and queue panel are filled by M5-62...65.
