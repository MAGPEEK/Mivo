# M3-12b Web: show, season and episode pages
Owner: mivo-frontend
Depends on: M3-12
Files/areas: `web/apps/web/src/features/item/**` (new `components/ShowPage.tsx`,
`SeasonPage.tsx`, `EpisodePage.tsx`, `ItemHeader.tsx`, `ChildrenList.tsx`; hooks
`useItemChildren.ts`; `api/childrenQuery.ts`; `lib/episodeLabel.ts`; locales; tests;
`ItemPage.tsx` dispatch by kind), `web/apps/web/src/app/router.ts` (only `validateSearch` of
the item route) and `router.test.tsx`

## Goal
A show opens a simple show page with its seasons, a season lists its episodes in order, and an
episode page links back to show and season and to the previous and next episode – the minimal
milestone-3 versions of the Plex-style detail pages. UX: **DT-20, DT-21, DT-22** (M3:
minimal), DT-24 (visible: one season), DT-06 (summary text).

## Acceptance criteria
- [ ] `/items/$itemId` renders by `kind`: movie (existing page + year, genres and summary when
      present), show, season, episode, extra (like a movie without children).
- [ ] Show page: poster, title, year, summary, **Play** (starts `playItemId`, disabled with a
      hint when absent), seasons as a list or grid of season posters with "Season 1 · 9
      episodes" (`Specials` for season 0, shown last as delivered by the server).
- [ ] Season page: season poster (server fallback to the show poster), link to the show,
      title, Play (`playItemId`), previous/next season arrows (`previousId`/`nextId`, hidden
      at the ends), episode list with 16:9 thumbnail or placeholder, `Episode 8` label (or the
      episode title alone when unnumbered), runtime from the primary media; ascending by
      default with a toggle kept in the URL (`?order=desc`, validated in `router.ts`); paging
      with the children endpoint's cursor ("Load more" like the M2 grid).
- [ ] Episode page: 16:9 still or placeholder, `S1 · E3` label (multi-episode `S1 · E1–E2`),
      air date (localised), links to show and season (from `ancestors`), previous/next episode
      arrows, then the existing technical chips, track list and Play of M2.
- [ ] Headings, links and arrow buttons have accessible names; arrows are links (they are
      navigations) with `aria-label`s like "Previous episode: <title>"; focus moves to the
      page heading after navigation as on the M2 item page.
- [ ] `en` and `de` strings for every new label, with plural forms for "N episodes"/"N seasons".
- [ ] Vitest with the fake API: each page kind, order toggle round trip through the URL,
      ends of the sibling arrows, a show with only specials, the 131-episode season renders
      the first page and loads more, missing artwork placeholders; axe checks clean.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
- Business rules stay in hooks and `lib/` (label formatting, order param), components render
  only (CLAUDE.md).
- No cast, extras row, backdrop or full detail layout (M5); keep the visual style of the M2
  item page.
- Reuse `Poster` from M3-12 for both aspects.
