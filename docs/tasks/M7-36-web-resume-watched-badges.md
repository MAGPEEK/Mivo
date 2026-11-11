# M7-36 Web: resume, mark as watched, badges (DT-05, LB-41, LB-42)
Owner: mivo-frontend
Depends on: M7-35
Files/areas: `web/apps/web/src/features/item/` (detail actions, overflow menu), the card badge slot in `web/apps/web/src/shared/ui` (progress bar, watched corner, unwatched count), `web/apps/web/src/features/library/` (card hover watched toggle), locale files

## Goal
Detail pages offer "Resume from 42:10" and "Play from start", items can be marked watched or
unwatched from the detail page, the overflow menu and the card hover, and cards show watched,
progress and unwatched-count badges, as drawn in `detail.html` and `browse.html`.

## Acceptance criteria
- [x] DT-05: with `viewerState.positionSeconds > 0` the primary button reads "Resume" with the
      formatted position and its split menu offers "Play from start"; otherwise "Play".
- [x] "Mark as watched"/"Mark as unwatched" on detail (movie, episode, show, season), in the
      item overflow menu and as the card hover toggle; optimistic update with rollback on error;
      list, detail and row queries are invalidated.
- [x] Badges (LB-42): watched check corner, progress bar on 2:3 and 16:9 cards, coral unwatched
      episode count on show/season cards; reusable `ProgressBar`/badge components for M7-37.
- [x] Tests: Vitest (`pnpm -C web test`): resume label formatting (≥ 1 h), split menu, toggle
      rollback, badge rendering per kind.
- [x] Gates: `pnpm -C web lint` and coverage pass.

## Notes
Items without `viewerState` (music, photos, extras) show none of these controls.

## Implementation notes (M7-36)
- Pure rules: `web/apps/web/src/shared/lib/watchState.ts` (badges, resume position, expected
  state). Reusable marks for M7-37: `ProgressBar`, `WatchedBadge`, `CountBadge` in
  `src/shared/ui/WatchBadges.tsx`; `WatchBadgeOverlay` (library feature) adds the translated
  names. `MediaCard` got an `actions` slot, `Menu` a `triggerClassName` for the split button.
- `useWatchedToggle` (library feature, used by the cards and the detail page): the expected
  state shows at once (detail cache patched, cards read the pending request), is rolled back on
  error, and after a success everything below the `libraries` and `items` cache keys is
  reloaded (lists, facets, Home and Overview rows, details, children).
- Servers before M7-23 answer 501 and send no `viewerState`: no badges and no Resume show;
  "Mark as watched" stays visible for movie, episode, show and season, and after the first 501
  the detail button is disabled with the reason and the card toggle is hidden.
- Resume passes whole seconds as `?t=` (M7-35); "Play from start" opens the player without it.
- M7-35 follow-up: `stop` and `pagehide` bypass the reporter's 2 s throttle (ADR 0032 §8).
- Deviations are in `BACKLOG.md` ("m7-36 notes").
