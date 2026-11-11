# M7-13 SQLite: restriction in rows, search, children, summaries, detail
Owner: mivo-backend
Depends on: M7-12
Files/areas: `internal/app/rows/ports.go`, `internal/app/search/ports.go`, `internal/app/library/ports.go` (restriction parameters only), `internal/adapters/sqlite/rows_*.go`, search reader, `catalog_reader*.go`, `listing_*.go` (shared predicate, counts, size sort), `folder_items.go`, `age_rating.go` and migration `00016` when the stored variant is chosen (see Notes), their tests

## Goal
Every other catalog read honours the same restriction, so a locked title appears in no row, no
search result, no children list and not by direct link (US-12). The restriction follows the
ancestor rule (ADR 0032 §7, amendment 2026-11-11): an item is visible only if it and every
ancestor pass the content limit, and nothing of a hidden ancestor leaks.

## Acceptance criteria
- [ ] Port requests gain an `access.Restriction`: rows `ItemRequest`/`EpisodeRequest`,
      `SummaryReader.ItemSummaries`, search query, children listing, item detail; the sqlite
      readers apply the M7-12 predicate.
- [ ] A hidden item read by id returns `domain.ErrNotFound`; a show's children omit hidden
      episodes; search suggestions and result tabs omit hidden items and their people-only hits
      stay (people carry no rating).
- [ ] **Ancestor rule in the one shared predicate** (M7-12's builder, so listing, facets,
      letters, folders and totals change with it): an item passes only if it and every
      ancestor pass max age and hide-unrated, each by its own effective bucket. The child's
      own rating still decides its own bucket (a higher-rated child is hidden on its own).
- [ ] Applied in every read of this task: rows, search (inside the FTS query), children,
      summaries, detail. A descendant of a hidden item answers `domain.ErrNotFound` by id.
- [ ] Counts and aggregates consider visible members only: `childCount` and `leafCount` in
      listings, children, summaries and detail; `episodeScope`/`libraryEpisodesSQL` (episode
      rows); `memberSizeSQL` (size sort and any size shown for a container). A show whose
      episodes are all hidden reports 0, not the real number.
- [ ] No leak of a hidden ancestor: no summary, detail, search hit or folder entry returned
      under a restriction contains the title, sort title, path segment or artwork reference of
      a hidden item (test: serialise every result of the fixture read and assert the hidden
      show's title and directory name are absent).
- [ ] Folder view on the sqlite side: `ListFolderItems` takes the restriction, and
      `CatalogReader.ListFolderDirectories(ctx, rootID, directory, restriction)` returns the
      names of the direct sub-directories that contain at least one visible item at any
      depth (add it if M7-12 did not, adapt it if it did). M7-14 adds it to the port.
- [ ] The two M7-12 folder-view tests that expect a lower-rated child to stay visible below
      a hidden parent are flipped: the child and its directory are absent.
- [ ] Tests: integration tests (`go test ./internal/adapters/sqlite/ -run 'Rows|Search|Children|Detail|Listing|Folder'`)
      on the M7-12 fixture, extended by: show FSK 16 with an FSK 0 episode; unrated show with
      an FSK 0 episode; FSK 0 show with an FSK 16 episode; a three-level chain
      (show → season → episode) with the rating on the season. For the "Children"
      restriction (max 6, hide unrated): no FSK 16 item, no descendant of a hidden item and
      no hidden title in any read; the same with hide-unrated off shows the unrated show and
      its FSK 0 episode; zero restriction yields nothing; `access.Unrestricted()` yields
      everything with unchanged counts.
- [ ] The listing benchmark does not regress by more than 10 % against M7-12 (`-bench Listing`).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
App services keep compiling by passing `access.Unrestricted()` until M7-14 threads the viewer
through; mark those call sites with `// TODO(M7-14)`. The FTS5 query must filter inside the
query (not after `LIMIT`), or pages come back short.

Ancestor rule, implementation hint (decide by the benchmark):
- Preferred: store the visibility per item so one indexed predicate still filters every
  query. One bucket "max over the ancestor chain" is **not** enough: with hide-unrated off
  an unrated ancestor must pass, with it on it must hide the subtree. Store two values:
  the maximum own bucket over the item and its ancestors (NULL when none is rated) and a
  flag "the chain has an unrated member" (true exactly when the top ancestor has no own
  rating). Predicate: bucket is NULL or ≤ max age, and (not hide-unrated or flag is 0).
  Maintain both where `effective_age_rating` is propagated today (same transaction, same
  subtree walk, the insert trigger, `BackfillAgeRatings`); stale rows must be recomputed by
  the backfill after the migration.
- That needs migration `00016` (M7-41 owns `00015` and merges first). Never edit `00014`.
- Alternative without a migration: a recursive ancestor check in the predicate. Only
  acceptable if the benchmark criterion holds.
- Reparenting (backlog "m7-11 review minors"): any write that changes `parent_id` must
  recompute the subtree.
- Out of scope here, stays with M7-14: the watch-store episode counts and `NextUp` (backlog
  "m7-18 review minors").
- Size: with the stored variant this may exceed ~400 lines. Then split off `M7-13a`
  (migration, propagation, predicate, flipped M7-12 tests, counts) before the remaining
  reads and tell the orchestrator.
