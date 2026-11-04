# M3-13b Contract: music kind, item kinds, disc number, square artwork
Owner: mivo-backend
Depends on: M3-19a, M3-19k (contract and `libraryKinds` exist)
Files/areas: `api/openapi.yaml`, `internal/adapters/http/apigen` (generated), `web/packages/api-client` (generated), `internal/adapters/http/restapi` (spec test only)

## Goal
The API can describe music libraries, albums and tracks; clients are regenerated.

## Acceptance criteria
- [ ] `LibraryKind` gains `music`; `ItemKind` gains `artist|album|track`; `ItemSummary`/`ItemDetail` gain optional `discNumber`; artwork kind `square`.
- [ ] `libraryKinds` stays unchanged here (the tile is enabled in M3-13g).
- [ ] `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean; the existing spec test passes.
- [ ] Tests: spec test asserts the new enum values.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Additive only; no handler changes. The image endpoint widths for `square` reuse the poster width set (ADR 0027 §8).
