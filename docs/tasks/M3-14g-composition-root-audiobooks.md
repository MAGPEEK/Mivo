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
