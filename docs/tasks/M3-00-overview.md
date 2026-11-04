# Milestone 3 "Bibliotheken komplett" – overview

Scope (plan, milestone 3; order 3 → 6 → 5 → 4 → 7 per OD-UX-2 B): name parser for movies and
shows with the absolute-numbering regression (UX DT-24), hierarchy show → season → episode with
simple show, season and episode pages (DT-20 … DT-22), local Kodi NFO metadata, local and
embedded artwork, an image endpoint with fixed sizes and a cache (LB-44) and posters in grid and
detail (LB-40), versions and extras folders, watcher and periodic scan with their config keys
(AD-23). Second wave: music tags, audiobook chapters, photo EXIF, disc images.

Not in scope: TMDB/MusicBrainz, the edit dialog and "correct match" (M6 – but the model leaves
room: provider IDs, field locks, provider/upload artwork sources, per-item metadata language);
the full detail layout, cast row, extras row, virtualised grid, filters (M5); version/audio/
subtitle selectors and generated episode stills (M4); watch state (M7, OD-UX-3); the TV app
(M10, OD-UX-1). Neither open decision OD-UX-1 nor OD-UX-3 is a dependency of any M3 task.

## ADR

| ADR | Decision | Status |
|---|---|---|
| [0027](../adr/0027-library-model-v2.md) | Library model v2: kinds, hierarchy, match keys and placement, parser rules incl. DT-24, migration `00003`, metadata merge with field locks, Kodi NFO, local artwork, FFmpeg-rendered image endpoint and cache, scanning v2, watcher and periodic scan, second-wave outline | Proposed (M3-01) |

## Numbering

The number is the area, a letter suffix a further one-sitting slice of the same area (as with
M2-24b): 02 contract, 03 naming, 04 domain, 05 sqlite, 06 read API, 07 local metadata sources,
08 FFmpeg images, 09 image service and endpoint, 10 scan, 11 triggers and wiring, 12 web,
17 corpus, 18 E2E. 13 … 16 are the second wave.

## Tasks – video slice

| ID | Title | Owner | Depends on | Wave | UX ids |
|---|---|---|---|---|---|
| [M3-01](M3-01-adr-library-model-v2.md) | ADR 0027 library model v2 (written) | mivo-architect | – | 0 | DT-24, LB-44, ED-02/05/07 room |
| [M3-02](M3-02-api-contract-library-v2.md) | API contract v2 + generated code + image URL helper | mivo-backend | 01 | 1 | LB-40, LB-44, DT-06, DT-08, DT-09, DT-20…22, DT-24 |
| [M3-04](M3-04-domain-catalog-model-v2.md) | Domain: item kinds, placement, file classes, sort titles | mivo-backend | 01 | 1 | DT-20…22, LB-22, DT-09 |
| [M3-04b](M3-04b-domain-metadata-and-artwork.md) | Domain: metadata merge + locks, artwork rules | mivo-backend | 01 | 1 | ED-02, ED-05, ED-07, LB-44 |
| [M3-05](M3-05-sqlite-catalog-v2-migration.md) | SQLite: migration `00003` catalog v2 | mivo-backend | 01 | 1 | (data) |
| [M3-11](M3-11-watcher-and-periodic-scan.md) | Watcher + periodic scan + config keys (go.mod: fsnotify) | mivo-backend | 01 | 1 | AD-23 |
| [M3-17](M3-17-testmedia-library-layouts.md) | Corpus: library layouts, NFO, artwork, DT-24 show | mivo-media | 01 | 1 | (verifies) |
| [M3-03](M3-03-naming-movies-versions-extras.md) | Naming: movies, versions, parts, extras | mivo-backend | 04 | 2 | DT-09, LB-42 data |
| [M3-06](M3-06-app-library-read-model-v2.md) | App: library read model v2 | mivo-backend | 04, 04b | 2 | DT-20…22, LB-40 |
| [M3-07](M3-07-nfo-reader.md) | Kodi NFO parser (go.mod: x/text) | mivo-backend | 04b | 2 | DT-06, DT-08, ED-07 |
| [M3-07b](M3-07b-libraryfs-sidecars.md) | FS: sidecars, NFC paths, bounded reads (go.mod: x/text) | mivo-backend | 04 | 2 | (LB-40 source) |
| [M3-08](M3-08-ffmpeg-image-renderer.md) | FFmpeg: artwork renderer | mivo-media | 04b | 2 | LB-44 |
| [M3-09](M3-09-app-images-and-cache.md) | Images: use case + disk cache | mivo-backend | 04b | 2 | LB-44 |
| [M3-12](M3-12-web-posters.md) | Web: posters in grid and detail | mivo-frontend | 02 | 2 | LB-40 |
| [M3-03b](M3-03b-naming-shows-episodes.md) | Naming: shows, seasons, episodes, **DT-24** | mivo-backend | 03 | 3 | DT-24, DT-20…22 |
| [M3-05c](M3-05c-sqlite-catalog-reader-v2.md) | SQLite: reader v2 + artwork lookup | mivo-backend | 05, 06, 09 | 3 | DT-20…22, DT-24, LB-40/44 |
| [M3-06b](M3-06b-restapi-items-v2.md) | REST: items v2 + children | mivo-backend | 02, 06 | 3 | DT-20…22, LB-40 |
| [M3-09b](M3-09b-http-image-endpoint.md) | HTTP: image endpoint | mivo-backend | 02, 09 | 3 | LB-44 |
| [M3-12b](M3-12b-web-show-season-episode-pages.md) | Web: show, season, episode pages | mivo-frontend | 12 | 3 | DT-20, DT-21, DT-22, DT-24 |
| [M3-10](M3-10-scan-v2-placement.md) | Scan v2: placement, versions, extras, port v2 | mivo-backend | 03, 03b, 04, 04b, 07, 07b | 4 | DT-20…22, DT-24, DT-09 |
| [M3-05b](M3-05b-sqlite-catalog-writer-placement.md) | SQLite: writer v2 placement + sidecars | mivo-backend | 05, 10 | 5 | DT-24 (data) |
| [M3-05d](M3-05d-sqlite-catalog-writer-metadata-artwork.md) | SQLite: writer v2 metadata + artwork | mivo-backend | 05, 10 | 5 | ED-02 guard, LB-40 data |
| [M3-10b](M3-10b-scan-v2-local-metadata-artwork.md) | Scan v2: NFO metadata + artwork | mivo-backend | 10 | 5 | LB-40, DT-06, DT-08 |
| [M3-11b](M3-11b-composition-root-library-v2.md) | Composition root + integration test | mivo-backend | 05b, 05c, 05d, 06b, 08, 09b, 10b, 11, 17 | 6 | checkpoint |
| [M3-18](M3-18-e2e-library-v2.md) | E2E: posters, show → season → episode, DT-24 | mivo-frontend | 11b, 12b, 17 | 7 | LB-40, LB-44, DT-20…22, DT-24 |

## Tasks – second wave (after M3-18 is green)

| ID | Title | Owner | Depends on | Gate |
|---|---|---|---|---|
| [M3-13](M3-13-music-library.md) | Music: artist → album → track | mivo-backend (+ mivo-media) | 11b, 18 | split into M3-13a … first |
| [M3-14](M3-14-audiobook-library.md) | Audiobooks with chapters | mivo-backend (+ mivo-media) | 13 | split first |
| [M3-15](M3-15-photo-library.md) | Photos with EXIF and thumbnails | mivo-media (+ mivo-backend) | 11b, OD-M3-4 | split first |
| [M3-16](M3-16-disc-images.md) | Disc images ISO/BDMV/VIDEO_TS | mivo-media | 11b, OD-M3-5 | split first |

The second-wave files fix scope and acceptance criteria; each spans several layers and is split
into one-sitting tasks by the architect once the video slice has merged (and, for 15/16, the
owner has answered the tooling decision). M3-13 and M3-15/M3-16 can then run in parallel;
M3-14 reuses M3-13's audio classification and covers.

## Waves (parallel within a wave; no two tasks of a wave share a file)

1. M3-02 ‖ M3-04 ‖ M3-04b ‖ M3-05 ‖ M3-11 ‖ M3-17 – go.mod: M3-11 only (fsnotify).
   M3-04 is additive so M3-05 compiles against either version of `domain/library`.
2. M3-03 ‖ M3-06 ‖ M3-07 ‖ M3-07b ‖ M3-08 ‖ M3-09 ‖ M3-12 – go.mod: M3-07 and M3-07b make the
   **identical** one-line change (promote `golang.org/x/text` to a direct requirement); the
   second to merge rebases without conflict. M3-07b adds one filter line to
   `internal/app/scan/run.go`; nobody else touches `app/scan` in this wave.
3. M3-03b ‖ M3-05c ‖ M3-06b ‖ M3-09b ‖ M3-12b – M3-05c builds its fixtures with plain SQL, so
   it does not wait for the writers.
4. M3-10 alone (declares the scan's v2 catalog port, which the writers and M3-10b need).
5. M3-05b ‖ M3-05d ‖ M3-10b – M3-05b owns `sqlite/library_catalog.go` (struct and
   constructor); M3-05d adds methods in its own files. Each owns its own `queries/*.sql` and
   the matching `sqlcgen/*.sql.go`; `sqlcgen/models.go` is changed only by M3-05 in this
   milestone's video slice.
6. M3-11b – **walking-skeleton checkpoint**: `go run -tags webui ./cmd/mivo` with a movies and
   a shows library shows posters and the hierarchy; the Go integration test over
   `testdata/library` passes, including DT-24.
7. M3-18, then the milestone summary for the owner and the release decision.

Shared files: `docs/tasks/BACKLOG.md` (each task deletes only its own rows, named in its
file); `web/apps/web/src/test/**` belongs to M3-12 in wave 2, M3-12b keeps new fakes inside
its feature. `go.sum` follows `go.mod`.

**Why the dual paths.** Every commit on `main` must build and scan. M3-10 therefore adds the
v2 scan pipeline behind an optional `CatalogV2` dependency and M3-06 the children reader as a
separate optional port; the milestone-2 paths keep running until M3-11b wires v2 and deletes
them. The cost is one wave of duplicated pipeline code in `app/scan`, removed in M3-11b.

**Why the skeleton is wired in wave 6.** As in milestone 2 the contract (M3-02) and the domain
(M3-04/04b) are fixed in wave 1, so every layer is tested against them with fakes from wave 2
on; a visible hierarchy needs the parser, the scan v2 and both writers anyway. Posters could be
wired earlier only with a second, throw-away wiring of `cmd/mivo`.

## Agents

| Agent | Tasks |
|---|---|
| mivo-architect | M3-01 (done), the split of M3-13 … M3-16, ADR 0027 Notes after deviations |
| mivo-backend | M3-02, M3-03, M3-03b, M3-04, M3-04b, M3-05, M3-05b, M3-05c, M3-05d, M3-06, M3-06b, M3-07, M3-07b, M3-09, M3-09b, M3-10, M3-10b, M3-11, M3-11b, M3-13, M3-14 |
| mivo-media | M3-08, M3-17, M3-15, M3-16 (and the probing/cover parts of M3-13, M3-14) |
| mivo-frontend | M3-12, M3-12b, M3-18 |
| mivo-webos | none in milestone 3 |

## Milestone 6 readiness (keep in mind for review)

| M6 need | Prepared by |
|---|---|
| Provider IDs (TMDB/IMDb/TVDB) per item, from NFO now | ADR 0027 §5, M3-04b, M3-05, M3-07, M3-05d |
| Field locks for the edit dialog (ED-02), never overwritten by a refresh | M3-04b (merge), M3-05 (table), M3-05d (guard), M3-10b (scan respects locks) |
| Artwork from providers and uploads, stored in the data directory (ED-05) | ADR 0027 §8, M3-04b (`SourceKind`), M3-05 (`artwork` columns), M3-09 (source switch) |
| Ordered metadata sources (AD-22) | M3-04b (`Merge` over an ordered list) |
| Per-item metadata language (ED-07) | M3-05 (`items.metadata_language`) |
| Absolute → provider episode mapping (DT-24 M6 part) | `items.absolute_number` (M3-05, M3-03b) |

## DT-24 regression ("Dragon Ball Super – 130 seasons")

Covered on every layer: parser table and the 131-name test (M3-03b), scan with fakes (M3-10),
sqlite writer (M3-05b) and reader ordering/paging (M3-05c), real-file integration test
(M3-11b, corpus from M3-17) and the browser (M3-18).

## BACKLOG rows picked up

`api` spec test (M3-02); `domain/library` nits and NFC (M3-04); `sqlite` nits of M2-10
(M3-05, rest M3-05c); `sqlite` cursor `\r\n` (M3-05c); `restapi library` (M3-06b); `app/scan`
M2-40 leftovers (M3-10); `web library` scan button (M3-12); `testmedia` nits optional (M3-17).

## Open decisions for the owner

Tasks follow the recommendation until answered (details in ADR 0027).

- **ADR 0027 acceptance** – the whole model; if rejected in parts, the affected tasks are
  re-planned before wave 1 starts.
- **OD-M3-1 Upgrade path** (M3-05): A drop the catalog and re-probe once (recommended) /
  B migrate rows in place.
- **OD-M3-2 Artwork resizing** (M3-08, M3-09): A FFmpeg (recommended) / B libvips now.
- **OD-M3-3 Scan defaults** (M3-11): watcher on, interval 12 h (recommended) / 6 h / 1 h.
- **OD-M3-4 Photo tooling** (M3-15 only; needed before the second wave).
- **OD-M3-5 Disc images and FFmpeg build flags** (M3-16 only; touches the Docker image and
  the SPK FFmpeg build).
- **Scope cut to confirm:** air-date episode names (`Show 2024-01-05`) are not parsed in M3;
  such files become unnumbered episodes (ADR 0027 §4).

## Approvals the orchestrator needs from the user

- Network: module download `github.com/fsnotify/fsnotify` (M3-11); promotion of
  `golang.org/x/text` (already in the module cache as an indirect dependency; M3-07, M3-07b);
  govulncheck database (all Go tasks).
- Docker: `sqlc/sqlc:1.31.1` for M3-05, M3-05b, M3-05c, M3-05d, M3-11b; the FFmpeg 7.1 image
  check of M3-08; the Linux watcher tests of M3-11.
- Release notes of the next version must announce the one-time catalog rebuild (re-probe of
  every file) – collected by M3-11b.

## Risks and notes

- The first scan after the upgrade re-probes every file; large NAS libraries need minutes to
  hours (two probes at a time).
- Item identity follows path keys: a renamed show or movie folder creates new items. Harmless
  now, relevant for watch state in M7 (OD-UX-3).
- inotify watch limits on NAS kernels and SMB/NFS mounts without events: the watcher degrades
  to the periodic scan with one warning per library.
- The image cache has no size cap in M3; it can be deleted at any time and is purged of stale
  entries at startup and after scans.
- 25 task files in the video slice: plan for several review rounds; the stop rule (3 failed
  rounds) applies per task as usual.
