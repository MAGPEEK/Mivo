# M5-66 Next-up card
Owner: mivo-frontend
Depends on: M5-65, M5-64
Files/areas: `web/apps/web/src/features/player/components/NextUpCard.tsx`, `hooks/useNextUp.ts`, `lib/nextUpTiming.ts`, locales

## Goal
In the last seconds of an item the player shows a card with the next queue item and, when Auto
play is on, a countdown that starts it (PL-08).

## Acceptance criteria
- [ ] Card appears for the last 30 s (or when a credits segment is present, M4: the timing
      function takes an optional segment); never for the last queue item without repeat.
- [ ] Countdown 10 s with Cancel and "Play now"; cancelling prevents auto start for this item.
- [ ] Auto play off: the card stays without countdown; at `ended` nothing starts automatically.
- [ ] Respects reduced motion; focus is not stolen from the controls.
- [ ] Tests: timing function table tests, hook with fake timers, ended-event behaviour.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Segment-based timing (skip credits) is M4; keep the input optional now.
