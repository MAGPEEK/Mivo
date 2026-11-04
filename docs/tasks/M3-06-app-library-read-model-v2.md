# M3-06 Use case: library read model v2 (hierarchy, metadata, artwork refs)
Owner: mivo-backend
Depends on: M3-04, M3-04b
Files/areas: `internal/app/library/{items.go,ports.go,children.go (new)}`, their tests and
`fakes_test.go`

## Goal
The catalog use cases return what the show, season and episode pages and the poster grid need:
top-level items, children of a container in either order, and an item detail with metadata,
ancestors, siblings, play target, extras and artwork references. Adapters (sqlite M3-05c, REST
M3-06b) implement and consume exactly these types. UX: DT-20 … DT-22, DT-24, LB-40, DT-06,
DT-08, DT-09 (data).

## Acceptance criteria
- [ ] `ItemSummary` v2: `library.Item` fields plus year, poster and thumb (`artwork.Ref`,
      optional), child and leaf counts, primary media summary (optional for containers).
- [ ] `ItemDetail` v2: item, `metadata.Record`, ancestors (`ItemRef{ID, Kind, Title,
      Number}`), previous/next sibling IDs, play item ID, artwork refs by kind, extras
      (`[]ItemSummary`), files (with version label and part number).
- [ ] `ItemReader` keeps `ListItems` (top-level only, doc updated) and `GetItem` (v2 detail);
      a **separate** port `ChildReader.ListChildren(ctx, itemID, ChildPageRequest{Page,
      Descending})` with an optional `Dependencies.Children`, so the M2 sqlite reader still
      satisfies `ItemReader` and `cmd/mivo` compiles until M3-05c/M3-11b. Error contract as
      in M2 (ErrInvalid only for a bad cursor, stored-row faults are plain errors) and
      documented ordering rules (ADR 0027, M3-02 contract).
- [ ] `Service.ListChildren` normalises the page like `ListItems`, returns `ErrNotFound` for
      an unknown item (the reader decides; the service passes it on), an empty page for a
      leaf, and `domain.ErrUnavailable` while no `ChildReader` is wired.
- [ ] `GetItem` and `ListItems` keep their M2 behaviour for movies (one file, no metadata) so
      the current web app works unchanged against fakes.
- [ ] Tests with in-memory fakes (no mock frameworks): ordering contract asserted on the fake
      the same way the sqlite reader must behave (specials last, unnumbered last, desc),
      unknown item, leaf, cursor errors, extras excluded from children. Coverage ≥ the app
      threshold.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/...`, coverage gate.

## Notes
- The fake must enforce the same ordering and error rules as the sqlite reader (lesson of
  M2-40); keep the rules in one helper of the fake and say in its doc comment which adapter
  behaviour it mirrors.
- `artwork.Ref` and `metadata.Record` come from the domain (M3-04b); do not define parallel
  DTOs here.
- Library availability and sync (M2) are unchanged.
- Changes to `ItemSummary`/`ItemDetail` are additive (new fields only), so the M2 sqlite
  reader in `adapters/sqlite` compiles unchanged until M3-05c replaces it.
