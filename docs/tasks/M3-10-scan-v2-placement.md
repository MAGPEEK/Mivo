# M3-10 Use case: scan v2 – placement, versions, extras, catalog port v2
Owner: mivo-backend
Depends on: M3-03, M3-03b, M3-04, M3-04b, M3-07, M3-07b
Files/areas: `internal/app/scan/**` (new `ports_v2.go`, `run_v2.go`, `placement.go`,
`sidecars.go`, `refresh.go`; `service.go` and `ports.go` only for the `CatalogV2`/`Finished`
dependencies; fakes and tests), `docs/tasks/BACKLOG.md` (only the `app/scan` rows of M2-40)

## Goal
A scan files every video into the hierarchy: movies with their versions, parts and extras,
shows with seasons and episodes – including absolute-numbered shows – and keeps item IDs
stable across rescans. It declares the complete v2 catalog port (placements, sidecars,
metadata and artwork) that the sqlite writers implement, so M3-05b, M3-05d and M3-10b can run
in parallel afterwards. UX: DT-20 … DT-22, **DT-24**, DT-09, LB-42 data; ADR 0027 §3, §9.

## Acceptance criteria
- [ ] New port `CatalogV2` = `PlacementCatalog` + `MetadataCatalog` (doc comments state every
      error like M2-40). `PlacementCatalog`: `PlacedFiles` (file states with stored placement
      key), `SavePlacedFile(ctx, PlacedFile{Placement, File, ProbedAt}, newID)` returning the
      chain's item IDs, `MoveFile(ctx, libraryID, fileID, placement, newID)`,
      `RemovePlacedFiles` (removes emptied ancestors), `SidecarStates`,
      `SaveSidecarStates(ctx, libraryID, upserts, removals)`. `MetadataCatalog`:
      `ItemMetadataState(ctx, itemID)`, `SaveMetadata(ctx, itemID, metadata.Record)`,
      `SaveArtwork(ctx, itemID, []artwork.Candidate, selection)`. M3-05b implements the first,
      M3-05d the second, on a **new** sqlite type, so the M2 `CatalogWriter` keeps working.
- [ ] **Dual path until M3-11b:** `Dependencies.CatalogV2` (optional) selects the v2 pipeline;
      without it the service runs the unchanged milestone-2 pipeline with `Catalog`. All M2
      tests stay green; `cmd/mivo` is not touched. M3-11b wires `CatalogV2` and deletes the
      M2 pipeline, the M2 port and its fakes.
- [ ] Ports for M3-10b declared here: `SidecarReader.ReadSidecar(ctx, root, relativePath,
      maxBytes)` (libraryfs, M3-07b) and `NFOParser.Parse(data, kind) (nfo-shaped result,
      error)` matching M3-07's `nfo.Result` through a small app-level type (app must not
      import adapters).
- [ ] Walk collects videos and sidecars separately; the scan counts videos per directory
      (excluding extras folders and samples) for `naming.Place`, and places **every** video
      on every scan. Samples and unplaceable files are counted as skipped with a debug log.
- [ ] New or changed files are probed (unchanged limit of two, ADR 0022) and saved with their
      placement; unchanged files whose placement key changed are moved without a probe; extras
      whose parent movie has no video are not saved (counted as skipped).
- [ ] Sidecar diff (size, mtime) is computed and stored; the set of **items to refresh**
      (new items, items with new/changed/removed files or sidecars in their folders) is
      handed to an extension point that M3-10b fills (a no-op in this task, tested).
- [ ] `Dependencies` gains an optional `Finished func(ctx context.Context, libraryID string)`
      called after every scan ends, on both paths (used by M3-11b for the image cache purge).
- [ ] The in-memory catalog fake enforces the port contract (chain upsert by key, ID
      stability, bottom-up removal, sidecar idempotence) like the sqlite writer must
      (lesson of M2-40); its rules are table-tested.
- [ ] Scenario tests with fakes: movie folder with two versions and a trailer folder; loose
      movies; parts; show with seasons, specials and a multi-episode file; renamed episode
      keeps its ID; folder rename moves files and removes the old show; **131 absolute-numbered
      files → 1 show, 1 season, 131 episodes (DT-24)**; rescan without changes probes and
      moves nothing; ADR 0022 safety rules (skipped paths, lost root) still hold.
- [ ] BACKLOG `app/scan` rows touched by this change are done and deleted (M2-40 leftovers:
      `ProbedFile` doc vs adapter rules, fake comment, `t.Run` in table loops).
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/...`, coverage gate.

## Notes
- Why the dual path: the sqlite adapter implements `CatalogV2` only after this task (M3-05b,
  M3-05d), and every commit on `main` must build and scan. The v2 pipeline lives in new files;
  shared helpers (probe limiter, tracker, failure codes) are reused, not copied.
- The v2 method names differ from the M2 ones on purpose, so one sqlite type could implement
  both if needed; M3-05b/M3-05d still use a new type.
- Keep functions small: placement, diff, probe and save are separate steps (files).
- Per-file logs stay at debug with relative paths only (ADR 0022).
- Deviation (review 1): `PlacementCatalog` is split into `PlacedFileCatalog` (PlacedFiles, SavePlacedFile, MoveFile, RemovePlacedFiles) and `SidecarCatalog` (SidecarStates, SaveSidecarStates); `CatalogV2` = `PlacedFileCatalog` + `SidecarCatalog` + `MetadataCatalog`. `MoveFile(ctx, MoveRequest{LibraryID, FileID, Placement, NewID})` takes a struct (4-parameter rule).
