# M3-15h Composition root: photos wiring, kind tile, integration test
Owner: mivo-backend
Depends on: M3-15b, M3-15c, M3-15d, M3-15e, M3-15f, M3-15g
Files/areas: `cmd/mivo`, `restapi` (`libraryKinds`, photo block mapping), config kind validation, integration test

## Goal
Photos work end to end on the server.

## Acceptance criteria
- [x] `libraryKinds` reports `photos`; item detail returns the photo block and `thumbnailState`.
- [x] Integration test over the corpus: album tree, EXIF values, thumbnail widths via the image endpoint (orientation applied), corrupt file listed with state, scan unaffected.
- [x] Tests: Go integration test; `go test -race -tags webui ./cmd/...`.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No Docker/SPK change is needed (ADR 0029).

Done in M3-15h (beyond the wiring in `cmd/mivo`: photo scan ports, photo renderer with
`DetectPhotoDecoders`, `photos` config kind):
- The generic item reads of `adapters/sqlite` serve photos (ADR 0029 "Photo API"): children of
  a photo album in album order (ascending only; `order=desc` answers 400, contract updated),
  summaries and detail with the photo details and the square tile, the JPEG of a RAW pair as
  primary file (`ListPrimaryFiles`, `ListItemFiles`), the listing kind condition for
  `photo_album`/`photo`. A video in a photos library has no tile yet; the album cover query
  skips videos.
- HEIF/AVIF tile grids render through `-filter_complex [0:g:0]` with a `-max_alloc` canvas
  bound; the corpus test no longer knows a render failure (BACKLOG m3-15g findings).
- Left in BACKLOG: timeline sort `takenAt` (M5), photo sizes without EXIF, scan minors
  (`app/scan` is changed by M3-16k at the same time), gen-testmedia minors.
