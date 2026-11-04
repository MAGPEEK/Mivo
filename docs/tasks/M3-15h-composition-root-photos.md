# M3-15h Composition root: photos wiring, kind tile, integration test
Owner: mivo-backend
Depends on: M3-15b, M3-15c, M3-15d, M3-15e, M3-15f, M3-15g
Files/areas: `cmd/mivo`, `restapi` (`libraryKinds`, photo block mapping), config kind validation, integration test

## Goal
Photos work end to end on the server.

## Acceptance criteria
- [ ] `libraryKinds` reports `photos`; item detail returns the photo block and `thumbnailState`.
- [ ] Integration test over the corpus: album tree, EXIF values, thumbnail widths via the image endpoint (orientation applied), corrupt file listed with state, scan unaffected.
- [ ] Tests: Go integration test; `go test -race -tags webui ./cmd/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No Docker/SPK change is needed (ADR 0029).
