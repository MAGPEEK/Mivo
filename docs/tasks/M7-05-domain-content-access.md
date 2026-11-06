# M7-05 Domain: content ratings, limits and viewer access
Owner: mivo-backend
Depends on: none
Files/areas: new `internal/domain/access/`

## Goal
One pure package turns stored rating texts into age buckets, defines content limit presets and
describes what a viewer may see, so SQL filters and use cases enforce the same rules (US-11,
US-12).

## Acceptance criteria
- [ ] `ParseRating(text string) (AgeBucket, bool)` maps the systems of ADR 0032 §7 (FSK and bare
      ages incl. `FSK 12`, `de/12`, `DE-12`, `Rated PG-13`, MPAA, US TV, BBFC) to 0/6/12/16/18;
      ages between buckets round up; unknown or empty → `false`.
- [ ] `ContentLimit` (max age or none, hide unrated) with presets `youngChildren` 0, `children` 6,
      `teens` 12, `adults` none, `custom`; `Stricter(a, b)` returns the combined limit;
      `Allows(bucket, known)` implements hide-unrated.
- [ ] `LibraryAccess` (`All` or a set of ids) with `Intersect` and `Allows(libraryID)`.
- [ ] `Viewer` (auth session id, account id, profile id, admin flag, effective library access,
      effective limit)
      and `Restriction` (the SQL-relevant part: library ids or all, max age, hide unrated) built
      by `Viewer.Restriction()`; the zero `Restriction` value denies everything (fail closed);
      an explicit `Unrestricted()` exists for admin viewers without limits.
- [ ] Tests: table-driven `go test -cover ./internal/domain/access/` ≥ 90 % with at least 40
      rating strings (case, spaces, prefixes, `NR`, `Unrated`, `TV-MA`, `18+`, `15`, `13`, `X`),
      intersection and stricter-limit tables.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Bare numbers are ages (DE/AT/CH/NL style); `PG` is US/UK 6. Keep the mapping table data-driven
(one slice of entries), so further countries are rows, not code. No dependency on
`domain/account` (plain id strings), so M7-04 and M7-05 run in parallel.
