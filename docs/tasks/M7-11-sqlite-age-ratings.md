# M7-11 SQLite: migration 00013 age ratings
Owner: mivo-backend
Depends on: M7-05; merges after M7-10 (migration order)
Files/areas: `internal/adapters/sqlite/migrations/00013_age_ratings.sql`, the metadata write path in `internal/adapters/sqlite/library_catalog_metadata*.go`, new `internal/adapters/sqlite/age_rating*.go`

## Goal
Every item carries the age bucket of its rating and the effective bucket its children inherit,
so content limits become one indexed predicate.

## Acceptance criteria
- [ ] Migration `00013` adds `age_rating` (nullable), `effective_age_rating` (nullable) and
      `age_rating_source` (the `content_rating` text the values were computed from) to `items`,
      plus an index usable by the listing filter.
- [ ] Every write of `content_rating` (scan/NFO merge, enrichment, edit) sets both columns via
      `access.ParseRating`; a show's change updates the effective bucket of its seasons and
      episodes that have no own rating, in the same transaction.
- [ ] An idempotent `BackfillAgeRatings(ctx)` recomputes rows where `age_rating_source` differs
      from `content_rating` (covers upgrades); it runs once at startup (wired in M7-25) and is
      cheap when nothing changed.
- [ ] Tests: integration tests (`go test ./internal/adapters/sqlite/ -run AgeRating`): backfill
      on an upgraded database, episode inherits show, episode with own rating keeps it, show
      rating change propagates, unknown rating stays NULL.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Adapters may call the pure domain parser (layer rule allows it). Do not change the listing
queries here (M7-12).
