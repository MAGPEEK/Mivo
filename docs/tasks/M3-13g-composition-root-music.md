# M3-13g Composition root: music wiring, kind tile, integration test
Owner: mivo-backend
Depends on: M3-13b, M3-13c, M3-13d, M3-13e, M3-13f
Files/areas: `cmd/mivo`, `internal/adapters/http/restapi` (`libraryKinds`), `internal/platform/config` (kind validation), integration test

## Goal
Music works end to end on the server: add a music library, scan, list albums, play a track.

## Acceptance criteria
- [x] `libraryKinds` reports `music` as available; config and the library service accept it.
- [x] Integration test over `testdata/library/music` asserts the manifest (artist/album/track counts, ordering, covers via the image endpoint).
- [x] A track starts a playback session and direct-plays (or transcodes to AAC when the profile lacks the codec); the decision engine handles audio-only sources. If it does not, fix it here and note it in ADR 0027 Notes.
- [x] Tests: Go integration test; `go test -race -tags webui ./cmd/...`.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Last Go task of the music chain; the web follows in M3-13h.
