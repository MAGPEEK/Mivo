# M5-44 Season and episode grid layout
Owner: mivo-frontend
Depends on: M5-33, M5-40
Files/areas: `web/apps/web/src/features/item/components/{ShowPage,SeasonPage,EpisodePage,ChildrenList}.tsx`, new `EpisodeGrid.tsx`, locales

## Goal
Upgrade the minimal M3-12b pages to the final layout: show page with a seasons row, season
page with the episodes as a 16:9 grid or list (view toggle and size slider as in the library),
episode page in the common layout (DT-20, DT-21, DT-22).

## Acceptance criteria
- [ ] Show page: common layout, Play starts `playItemId`, seasons as a `MediaRow` with
      "Season 1 . 9 episodes", specials last.
- [ ] Season page: season poster with play overlay, show link, previous/next season arrows,
      episodes as grid (title, `Episode 8`, runtime) or list; ascending default with the
      `?order=desc` toggle retained from M3-12b.
- [ ] Episode page: 16:9 still, `S1 . E3`, air date, show/season links, previous/next arrows,
      technical chips and track list unchanged.
- [ ] The 131-episode season still pages with the cursor (virtualised via the M5-34 hook if
      > 200 episodes).
- [ ] Tests: M3-12b test cases stay green; new tests for view toggle and rows.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Watched toggle, unwatched badge and progress are M7 slots only.
