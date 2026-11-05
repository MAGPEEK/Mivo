# M3-14g Composition root: audiobooks wiring, kind tile, integration test
Owner: mivo-backend
Depends on: M3-14b, M3-14c, M3-14d, M3-14e, M3-14f
Files/areas: `cmd/mivo`, `restapi` (`libraryKinds`, `chapters` mapping), config kind validation, integration test

## Goal
Audiobooks work on the server end to end.

## Acceptance criteria
- [ ] `libraryKinds` reports `audiobooks`; item detail returns `chapters`.
- [ ] Integration test over the corpus asserts books, authors, chapter titles and offsets.
- [ ] A book file plays through the existing playback path.
- [ ] Tests: Go integration test; `go test -race -tags webui ./cmd/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Per-user resume is M7; do not add progress storage.

## Notes (M3-14b, contract 1.8.0)
- `ItemChapter.fileId` is required: a chapter from a single-file M4B points at that (primary)
  file, a part-file chapter at its part. `title` has `maxLength: 500` – cut longer tag titles.
- The audiobook cover is `poster` / `artwork.poster` (2:3, owner-approved media.html), not
  `square`; the scan stores book covers as `artwork.KindPoster`.
- `media.durationSeconds` and the `duration` sort of an audiobook are the sum of its part
  files (single-file books: the primary file).
- M3-14b already maps item kind `audiobook`, library kind `audiobooks` and the roles
  `author`/`narrator` in `restapi`; the listing `type` filter (`items_browse.go`) still
  accepts only `movie`/`show` and needs `audiobook`.
