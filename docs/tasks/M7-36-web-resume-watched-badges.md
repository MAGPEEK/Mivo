# M7-36 Web: resume, mark as watched, badges (DT-05, LB-41, LB-42)
Owner: mivo-frontend
Depends on: M7-35
Files/areas: `web/apps/web/src/features/item/` (detail actions, overflow menu), the card badge slot in `web/apps/web/src/shared/ui` (progress bar, watched corner, unwatched count), `web/apps/web/src/features/library/` (card hover watched toggle), locale files

## Goal
Detail pages offer "Resume from 42:10" and "Play from start", items can be marked watched or
unwatched from the detail page, the overflow menu and the card hover, and cards show watched,
progress and unwatched-count badges, as drawn in `detail.html` and `browse.html`.

## Acceptance criteria
- [ ] DT-05: with `viewerState.positionSeconds > 0` the primary button reads "Resume" with the
      formatted position and its split menu offers "Play from start"; otherwise "Play".
- [ ] "Mark as watched"/"Mark as unwatched" on detail (movie, episode, show, season), in the
      item overflow menu and as the card hover toggle; optimistic update with rollback on error;
      list, detail and row queries are invalidated.
- [ ] Badges (LB-42): watched check corner, progress bar on 2:3 and 16:9 cards, coral unwatched
      episode count on show/season cards; reusable `ProgressBar`/badge components for M7-37.
- [ ] Tests: Vitest (`pnpm -C web test`): resume label formatting (≥ 1 h), split menu, toggle
      rollback, badge rendering per kind.
- [ ] Gates: `pnpm -C web lint` and coverage pass.

## Notes
Items without `viewerState` (music, photos, extras) show none of these controls.
