# M5-65 Queue panel and filling the queue
Owner: mivo-frontend
Depends on: M5-60, M5-61
Files/areas: `web/apps/web/src/features/player/components/QueuePanel.tsx`, `features/player/hooks/usePlayerQueue.ts`, `features/queue/` (new public API `enqueue`, `playNext`, `playAll`), locales; call sites in `item` (Play on show/season)

## Goal
A queue panel (current item marked, durations, remove, drag/keyboard reorder, "Clear upcoming")
and the ways to fill it: Play on a show/season queues its episodes from the start point, Play
next / Add to queue from menus, Play all / Shuffle from the library list (PL-32, PL-33).

## Acceptance criteria
- [ ] Panel reorders by drag and by move buttons; live region announces changes; jumping to an
      item starts it.
- [ ] Play on a season queues episodes in order starting at the chosen one; on a show it
      starts at `playItemId`.
- [ ] Play all / Shuffle queue the current filtered list by paging the listing endpoint
      (cap of 1,000 items documented, with a notice when exceeded).
- [ ] Queue persists per tab session (sessionStorage) and survives player route changes.
- [ ] Tests: fill functions with fake API, panel keyboard and drag-fallback, persistence.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Server-side queue per profile is M7. Other features use only the `features/queue` public API.
