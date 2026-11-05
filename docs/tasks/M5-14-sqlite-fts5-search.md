# M5-14 SQLite FTS5 index and query
Owner: mivo-backend
Depends on: M3-05c
Files/areas: `internal/adapters/sqlite/` (migration, index maintenance, query), `internal/app/` (search port declaration only)

## Goal
Fast, accent- and case-insensitive prefix search over titles, original titles, sort titles,
people and genres (SR-03).

## Acceptance criteria
- [ ] Migration creates an FTS5 table (`unicode61 remove_diacritics`) kept in sync by triggers
      or writer hooks on insert, update and delete; an existing catalog is back-filled.
- [ ] `Search(ctx, query, filter, limit)` port (declared in `app`) returns ranked hits with
      type and library; prefix matching ("matr" finds "The Matrix"); "cafe" finds "Café".
- [ ] Hostile input (quotes, `*`, `NEAR`, empty, 10 kB) is escaped and never causes an SQL error.
- [ ] Integration tests on temp SQLite: rank order, delete/rename updates, 50,000 items query
      < 100 ms (benchmark number documented).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
People/genre columns exist only where M3 NFO delivered them; index what is there.

## Result (implementation notes)
- Migration `00010_search_index.sql`: contentful FTS5 table `item_search` (title, original
  title, sort title, people, genres; `unicode61 remove_diacritics 2`, prefix indexes 2 and 3),
  `search_docs` (stable `doc_id` per indexed item plus copies of `library_id` and `kind`, cascades
  with the item) and the view `search_doc_text`, the single definition of a document's text.
  Triggers on `items`, `item_people`, `people`, `item_tags` and `tags` keep the index in sync for
  every writer; the migration back-fills an existing catalog.
- Indexed kinds: movie, show, episode, artist, album, track, audiobook, photo_album, photo (the
  types M5-15 returns). Seasons and extras are not indexed. Genres are tags of kind `genre` only.
- Port `internal/app/search.Index` (`Search(ctx, query, filter, limit)`, `Filter{LibraryIDs,
  Kinds}`, `Hit{ItemID, LibraryID, Kind}`, `MaxLimit` 100). An empty `LibraryIDs` finds nothing.
  Adapter `sqlite.SearchIndex` (not wired yet; M5-15 wires it in `cmd/mivo`).
- Query text: words are runs of letters, digits and combining marks; every other character
  separates words, so no FTS5 syntax survives. Each word becomes a quoted prefix term, all
  words must match; at most 8 distinct words of at most 64 runes each.
- Ranking: `bm25` weights title 10, original title 5, sort title 5, people 2, genres 1; ties by
  `doc_id` (indexing order). A sort-title tie-break would join `items` for every match and
  doubled the worst case.
- Benchmark `BenchmarkSearch` (`go test -run '^$' -bench BenchmarkSearch -benchtime 20x
  ./internal/adapters/sqlite/`), 50,000 movies, limit 8, Intel Core Ultra 7 265K, Windows,
  shared with other test runs: "movie" (every item matches, worst case) 55 ms; "dra" (a genre
  prefix, a quarter of the items) 18 ms; "movie 4242" 7 ms; "comedy movie 12" 8 ms; no match
  0.2 ms. Target < 100 ms met.
- Write cost: an FTS5 write inside a trigger is slower than a bulk insert. Seeding the
  50,000-movie benchmark library (item, two genres, file, two streams per movie, one
  transaction) went from about 10 s to about 65 s; the migration back-fill of the same catalog
  takes under 1 s. Against ffprobe per file this is a few percent of a video or music scan
  (estimate, not measured in a real scan); photo scans probe no file with ffprobe, so there the
  share is larger.
- `BenchmarkSaveMetadataWithCredits` (`-benchtime 200x`): SaveMetadata of a movie with 30
  credits and two genres takes 6.4 ms with the index and 1.4 ms without it. Every credit row
  rewrites the FTS row (delete and insert of the credits). Against a provider lookup per item
  this is small; a statement-level reindex is a possible later optimisation.
- `BenchmarkDeleteLibraryWithCredits` (`-benchtime 3x`): deleting a library of 2,000 movies with
  30 credits and two genres each takes 286 ms with the index and 226 ms without it. The
  delete triggers of `item_people` and `item_tags` skip rows whose item is already gone, so a
  cascade does not rewrite FTS rows of items that are being deleted.
