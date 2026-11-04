# M3-10b Use case: scan v2 – local NFO metadata and artwork
Owner: mivo-backend
Depends on: M3-10 (M3-07, M3-07b, M3-04b through it)
Files/areas: `internal/app/scan/{metadata.go,artwork.go}` (new) + tests, the extension point
of M3-10 in `run.go`, `fakes_test.go` (only additions for NFO/sidecar fakes)

## Goal
After placing files, the scan reads each refreshed item's NFO, merges it with the file-name
data under the item's field locks, and selects local or embedded artwork, so movies and shows
show their own titles, summaries, genres, people and posters. UX: LB-40, DT-06, DT-08 (names),
ED-02 (locks respected); ADR 0027 §6–§8.

## Acceptance criteria
- [ ] For each item to refresh: find its NFO among the sidecars by the rules of ADR 0027 §7
      (movie: `<base>.nfo`, `movie.nfo` for folder movies; show: `tvshow.nfo`; season:
      `season.nfo`; episode: `<base>.nfo`, multi-episode records by episode number), read it
      with `ReadSidecar` (1 MiB limit), parse it, and build the `filename` source from the
      placement (title, year).
- [ ] `metadata.Merge(current, locks, [nfo, filename])` decides; the result is saved with
      `SaveMetadata` only when it differs from the stored record.
- [ ] Artwork: candidates from the sidecars with `artwork.ClassifyLocal` for the item's
      context (folder movie, loose movie, show folder incl. `seasonNN-poster` for seasons,
      season folder, episode) and from attached-picture streams of the item's files;
      `artwork.Select` per kind with the kind's lock; tags computed with `artwork.Tag`;
      saved with `SaveArtwork` only when changed.
- [ ] A missing, unreadable, oversized or malformed NFO never fails the scan: it is logged at
      debug with the relative path and the parser's warning code, and the item keeps its
      file-name data. NFO warnings are counted in a new scan count `metadataWarnings`
      (in-memory status only; contract change not needed until M5).
- [ ] Tests with fakes: NFO title wins over the folder name; locked title and locked poster
      survive an NFO change; season poster from `season01-poster.jpg` in the show folder;
      loose movie ignores a folder-wide `poster.jpg`; embedded cover used only without local
      poster; changed NFO (mtime) triggers a refresh, unchanged does not; malformed NFO;
      multi-episode NFO; scan cancellation during the metadata step ends with
      `scan_canceled`.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/...`, coverage gate.

## Notes
- Metadata reading runs after all files of the scan are placed and uses no probe slots; read
  NFOs sequentially (NAS disks) – a concurrency limit is not needed in M3.
- Do not write any file and do not fetch any URL (ADR 0010, ADR 0022).
- Image dimensions stay 0 until the first render; the scan never decodes images.
- Duty (M3-10 review): the refresher also refreshes requested items that have no stored metadata (`ItemMetadataState`), because a scan that ended early may have saved items whose refresh failed; and it always receives the full walked sidecar set in `RefreshRequest.Sidecars`, also from an early-ended scan.

- Known limitations (review 1): `metadata.Record` carries no episode number, so a multi-episode NFO (several records) gives the item its first record instead of the one matching its number. Unselected embedded covers of unchanged files are dropped on refresh. An NFO that is forbidden, malformed or oversized keeps the stored record; one that is unavailable or vanished fails the refresh so it is retried.
