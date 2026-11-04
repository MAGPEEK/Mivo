# M5-12 Contract: home and overview rows
Owner: mivo-backend
Depends on: M3-06b
Files/areas: `api/openapi.yaml`, generated code, `docs/adr/` (row model ADR if needed)

## Goal
One endpoint family returns row definitions with their first page of items (HO-07), for Home and
for a library's Overview pivot, so the client renders rows without computing them.

## Acceptance criteria
- [ ] `GET /api/v1/home/rows` and `GET /api/v1/libraries/{id}/rows` return
      `{id, kind, titleKey/params, seeAllQuery, items[], cardFormat}`; row kinds for M5:
      recentlyAdded, newReleases, moreGenre(genre), recentEpisodes; the kind set is an enum
      that M6/M7 extend (people, topRated, continueWatching, upNext).
- [ ] `seeAllQuery` is a Browse-pivot query (sort + filter) from M5-10, validated by the spec.
- [ ] Episode grouping: a show card with `newEpisodeCount`, or a season card (HO-05).
- [ ] Home response also carries a `hero` list (up to 5 items with backdrop image id, title,
      summary line; only items with artwork) for M5-26.
- [ ] Empty libraries return no rows plus a `hint` code (LB overview hint).
- [ ] Tests: spec validates; generated clients regenerate with a clean diff.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Titles are keys + params, not translated strings (server has no UI language). "More <genre>"
picks genres daily-rotating: define the seed (date) in the contract text.
