# M3-02 API contract v2: hierarchy, local metadata, artwork and images
Owner: mivo-backend
Depends on: M3-01
Files/areas: `api/openapi.yaml`, `api/spec_test.go`, `internal/adapters/http/apigen/**`
(regenerated + `oapi-codegen.yaml`), `internal/adapters/http/restapi/items.go` (only a 501 stub
for the new operation) and its test, `web/packages/api-client/src/generated/**` (regenerated),
`web/packages/api-client/src/{schemas.ts,index.ts,images.ts,images.test.ts}`,
`docs/tasks/BACKLOG.md` (only the `api` spec-test row)

## Goal
The milestone-3 API is specified first (ADR 0017), so backend, frontend and E2E tasks work
against generated Go stubs and a generated TypeScript client from wave 2 on. Items carry their
kind, place in the hierarchy, local metadata and artwork references; containers list their
children; posters are fetched from a fixed-size image endpoint. UX: LB-40, LB-44, DT-06,
DT-08 (names), DT-09 (data), DT-20 … DT-22, DT-24.

## Acceptance criteria
- [ ] `info.version` is `1.2.0`. Every new enum is documented as extensible ("clients must
      tolerate new values").
- [ ] `ItemKind` enum `movie|show|season|episode|extra`; `ExtraType` enum (ADR 0027 §4);
      `ImageRef` `{id (uuid), tag (string, opaque)}`; `ImageWidth` integer enum
      `160|320|480|640|960|1280|1920`; `ArtworkKind` `poster|backdrop|thumb|logo|banner`;
      `PersonRole` `actor|director|writer|producer|guest_star`; `ProviderIds` object with
      optional `tmdb`, `imdb`, `tvdb` strings.
- [ ] `ItemSummary` gains required `kind` and optional `year`, `parentId`, `number`,
      `numberEnd`, `absoluteNumber`, `extraType`, `childCount` (show: seasons, season:
      episodes), `leafCount` (show: episodes), `poster`, `thumb` (`ImageRef`). `media` stays
      optional (containers have none).
- [ ] `ItemDetail` gains required `kind` and optional `originalTitle`, `year`, `releaseDate`
      (`format: date`), `summary`, `tagline`, `contentRating`, `genres`, `studios`,
      `countries` (string arrays), `people` (`{name, role, character?}`, ordered), `providerIds`,
      `artwork` (object keyed by `ArtworkKind` → `ImageRef`; a season without a poster carries
      its show's poster), `ancestors` (`[{id, kind, title, number?}]`, root first),
      `previousId`/`nextId` (siblings of a season or episode in ascending order),
      `playItemId` (the item Play starts: itself for a playable leaf, the first episode of the
      first regular season for a show, the first episode for a season), `extras`
      (`ItemSummary[]`, `maxItems: 100`) and `childCount`/`leafCount`. `MediaFile` gains
      optional `versionLabel` and `partNumber`.
- [ ] `GET /api/v1/items/{itemId}/children` (`listItemChildren`, tag `items`): `limit` 1–500
      default 100, opaque `cursor`, `order` `asc|desc` default `asc`; returns `ItemPage`.
      Seasons ascend by number with specials (0) last; episodes by number, unnumbered last by
      sort title; extras are never children. Unknown item → 404 `not_found`; a leaf → empty
      page.
- [ ] `listLibraryItems` and `Library.itemCount` are documented as top-level items only
      (movies, shows); extras are never listed.
- [ ] `GET /api/v1/images/{imageId}` (`getImage`, new tag `images`, `security: []` like the
      stream routes): query `width` (required, `ImageWidth`) and `tag` (optional, ≤ 64 chars,
      pattern `^[0-9a-f]+$`); responses `200` `image/jpeg`/`image/png` with `ETag` and
      `Cache-Control` headers, `304`, `default` problem (400 `invalid`, 404 `not_found`,
      422 `unsupported` for an undecodable source, 503 `unavailable`). The description states
      the caching rule of ADR 0027 §8 and that clients build the URL only through the
      api-client helper.
- [ ] `oapi-codegen.yaml` excludes `getImage` next to the stream operations; `api/spec_test.go`
      checks that the excluded list equals the operations of the tags `stream` and `images`
      (and still that every other operation has a `default` problem response). The BACKLOG
      `api` row (`\b` in `TestNoValueIsTakenFromStreamURL`, scan response and header
      descriptions) is done and deleted.
- [ ] `restapi` answers `listItemChildren` with 501 and the internal `not_implemented` code
      until M3-06b (same mechanism as M2-01); a table test pins it.
- [ ] api-client exports the new schema types and a hand-written
      `imageUrl(ref: ImageRef, width: ImageWidth): string` (root-relative,
      `/api/v1/images/<id>?width=<w>&tag=<tag>`, every part URL-encoded) plus
      `imageSrcSet(ref, widths)`; Vitest covers encoding and the width union.
- [ ] `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean.
- [ ] Tests: `go test ./api/... ./internal/adapters/http/...`, `pnpm -C web test`.
- [ ] Gates: Go gates (vet, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./...`, coverage gate), `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- Adding required fields to responses is compatible for clients; the existing restapi
  mappers must fill `kind` now (from `library.ItemKind`, which still yields `movie|episode` in
  milestone 2 data) so the generated types compile – keep that change in `items_response.go`
  to one line per mapper and leave all other new fields empty until M3-06b.
- Do not add `music`/`audiobooks`/`photos` to `LibraryKind` here; M3-13 … M3-15 extend the
  contract with their kinds.
- `ImageRef.tag` is opaque for clients; only the helper puts it into the URL.
- The web app keeps compiling: new fields are optional except `kind`, which no web code reads
  yet.
