# M5-60 player-core queue model
Owner: mivo-frontend
Depends on: none
Files/areas: `web/packages/player-core/src/queue/**`, `src/index.ts`

## Goal
A framework-free queue (items, current index, repeat off/all/one, shuffle) in `player-core`
that the player, mini player, music bar and audiobook UI share (PL-31).

## Acceptance criteria
- [ ] Operations: set, playNext, add, remove, move, clearUpcoming, next, previous, jumpTo; all
      pure/immutable, return a new state.
- [ ] Repeat one stays on the item; repeat all wraps; shuffle gives a deterministic order for a
      seed, keeps the current item first, and un-shuffling restores the original order.
- [ ] `next` at the end without repeat returns "ended"; `previous` after 3 s signals "restart
      current" via a flag (rule documented in the file comment).
- [ ] State serialises to JSON and back (per-tab session persistence; invalid JSON yields an
      empty queue).
- [ ] Tests: table-driven, coverage >= 90 % for the folder, includes duplicate items and
      removing the current item.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, no DOM/React imports (dependency-cruiser).

## Notes
Item identity is an id string plus opaque metadata type parameter; no API types in the package.
