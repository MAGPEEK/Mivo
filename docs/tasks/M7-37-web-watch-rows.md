# M7-37 Web: Continue watching and Up next rows (HO-03, HO-04, LB-01)
Owner: mivo-frontend
Depends on: M7-36
Files/areas: `web/apps/web/src/features/home/`, the Overview rows in `web/apps/web/src/features/library/`, locale files

## Goal
Home starts with "Continue watching" (progress bars, `S3 · E9` labels) and "Up next"; a
library's Overview starts with its own "Continue watching"; the overflow menu removes an entry
or marks it watched.

## Acceptance criteria
- [x] Row kinds `continueWatching` and `upNext` render with the existing `MediaRow`, 2:3
      posters with the M7-36 progress bar; episodes show show title, episode title and the
      compact `S3 · E9` label.
- [x] Card click on Continue watching starts playback at the position (via the M7-35 start
      parameter); the overflow menu has "Remove from continue watching" and "Mark as watched",
      both refresh the rows.
- [x] Rows are hidden when empty (as the server omits them); no layout jump on Home.
- [x] Tests: Vitest (`pnpm -C web test`): rendering per kind, label format, remove action
      refetch.
- [x] Gates: `pnpm -C web lint` and coverage pass.

## Notes
Row titles come from `titleKey` (server); add the keys to en/de.
