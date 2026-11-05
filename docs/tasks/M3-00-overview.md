# Milestone 3 "Bibliotheken komplett" – overview

Scope (plan, milestone 3; order 3 → 6 → 5 → 4 → 7 per OD-UX-2 B): name parser for movies and
shows with the absolute-numbering regression (UX DT-24), hierarchy show → season → episode with
simple show, season and episode pages (DT-20 … DT-22), local Kodi NFO metadata, local and
embedded artwork, an image endpoint with fixed sizes and a cache (LB-44) and posters in grid and
detail (LB-40), versions and extras folders, watcher and periodic scan with their config keys
(AD-23). Second wave: music tags, audiobook chapters, photo EXIF, disc images.

Added by the owner on 2026-11-04: **library management in the app** – Settings › Libraries
with add / edit / remove, a server-side folder browser, several folders per library, metadata
language, the M3 advanced options, a scan after adding, a local-network-only guard until
accounts exist (AD-40, LS-01 … LS-03, M3-19 and its slices). It lets the release after
0.1.0-alpha.2 drop the folder question of the Synology install wizard (SPK-17).

Not in scope: TMDB/MusicBrainz, the edit dialog and "correct match" (M6 – but the model leaves
room: provider IDs, field locks, provider/upload artwork sources, per-item metadata language);
the full detail layout, cast row, extras row, virtualised grid, filters (M5); version/audio/
subtitle selectors and generated episode stills (M4); watch state (M7, OD-UX-3); the TV app
(M10, OD-UX-1). Neither open decision OD-UX-1 nor OD-UX-3 is a dependency of any M3 task.

## ADR

| ADR | Decision | Status |
|---|---|---|
| [0027](../adr/0027-library-model-v2.md) | Library model v2: kinds, hierarchy, match keys and placement, parser rules incl. DT-24, migration `00003`, metadata merge with field locks, Kodi NFO, local artwork, FFmpeg-rendered image endpoint and cache, scanning v2, watcher and periodic scan, second-wave outline | Proposed (M3-01) |
| [0029](../adr/0029-second-wave-tooling-photos-and-discs.md) | Photo tooling (OD-M3-4) and disc images (OD-M3-5) without new bundled binaries | Accepted |
| 0028 (written, accepted) | Library management in the app: database as source of truth, configuration as one-time import seed, several folders per library, browse roots and install kind, management guard (local peers, same origin) until M7; supersedes parts of ADR 0022 | Pending – mivo-architect writes it from the M3-19 Notes before lane L1 starts |

## Numbering

The number is the area, a letter suffix a further one-sitting slice of the same area (as with
M2-24b): 02 contract, 03 naming, 04 domain, 05 sqlite, 06 read API, 07 local metadata sources,
08 FFmpeg images, 09 image service and endpoint, 10 scan, 11 triggers and wiring, 12 web,
17 corpus, 18 E2E. 13 … 16 are the second wave. 19 is library management: `M3-19` is the
parent file (scope and decisions), `M3-19a` … `M3-19o` are its one-sitting slices.

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

## Tasks – library management (owner decision 2026-11-04, after the data model tasks)

Parent: [M3-19](M3-19-library-management-ui.md) (scope, persistence decision, security, open
decisions OD-M3-6/7). UX ids: AD-40, LS-01, LS-02, LS-03 (M3 subset), SH-18 (entry), HO-09.

| ID | Title | Owner | Depends on | Lane |
|---|---|---|---|---|
| [M3-19a](M3-19a-api-contract-library-management.md) | Contract: library CRUD, folder roots/listing + generated code | mivo-backend | 02, ADR 0028 | L1 |
| [M3-19b](M3-19b-domain-library-management.md) | Domain: library v2, folder rules, browse name policy | mivo-backend | 04, ADR 0028 | L1 |
| [M3-19c](M3-19c-config-browse-roots-and-library-seed.md) | Config: `browse.roots`, `install.kind`, `libraries` as seed | mivo-backend | 11, ADR 0028 | L1 |
| [M3-19d](M3-19d-fs-folder-browser.md) | FS: folder browser and folder check | mivo-backend | 19b | L2 |
| [M3-19e](M3-19e-app-library-management.md) | App: create/update/remove/import/browse | mivo-backend | 19b, 06 | L2 |
| [M3-19f](M3-19f-sqlite-migration-library-folders.md) | SQLite: migration `00004` folders, options, import ledger | mivo-backend | 11b | L3 |
| [M3-19i](M3-19i-scan-multi-folder-and-options.md) | Scan: several folders, options, cancel | mivo-backend | 19b, 11b | L3 |
| [M3-19j](M3-19j-scantrigger-dynamic-libraries.md) | Scan triggers: dynamic libraries, watch per folder | mivo-backend | 19b, 11b | L3 |
| [M3-19k](M3-19k-http-library-management.md) | HTTP: management handlers + guard | mivo-backend | 19a, 19e, 11b | L3 |
| [M3-19m](M3-19m-web-settings-libraries.md) | Web: Settings › Libraries, settings entry, empty states | mivo-frontend | 19a, 12b | L3 |
| [M3-19g](M3-19g-sqlite-library-store-v2.md) | SQLite: library store v2 | mivo-backend | 19e, 19f | L4 |
| [M3-19h](M3-19h-sqlite-multi-folder-catalog.md) | SQLite: catalog per folder | mivo-backend | 19f, 19i | L4 |
| [M3-19n](M3-19n-web-library-dialog-folder-browser.md) | Web: add/edit dialog, folder browser, permission help | mivo-frontend | 19m | L4 |
| [M3-19l](M3-19l-composition-root-library-management.md) | Composition root, Docker defaults, integration test | mivo-backend | 19c, 19d, 19g, 19h, 19i, 19j, 19k | L5 |
| [M3-19o](M3-19o-e2e-library-management.md) | E2E: empty state → add → scan → edit → remove | mivo-frontend | 19l, 19n, 18 | L6 |

## Tasks – second wave (after M3-18 and M3-19l are green)

| ID | Title | Owner | Depends on | Gate |
|---|---|---|---|---|
| [M3-13](M3-13-music-library.md) | Music (parent) | – | – | split 2026-11-04 |
| [M3-14](M3-14-audiobook-library.md) | Audiobooks (parent) | – | – | split 2026-11-04 |
| [M3-15](M3-15-photo-library.md) | Photos (parent) | – | – | split 2026-11-04 |
| [M3-16](M3-16-disc-images.md) | Disc images (parent) | – | – | split 2026-11-04 |

Baseline for all: video slice (M3-04 domain v2, M3-05 migration `00003`, M3-05c reader v2,
M3-10/10b scan v2, M3-11b wiring) and library management (M3-19b/h/i/k/l; kinds via
`libraryKinds`; migration `00004`). Decisions OD-M3-4/5: [ADR 0029](../adr/0029-second-wave-tooling-photos-and-discs.md).
Migrations: online metadata (M6-03) `00005`, music `00006`, audiobooks `00007`, photos `00008`; discs none. Always take the next free number at merge.

| Chain | Tasks (in dependency order) | Owners |
|---|---|---|
| Music | [13a](M3-13a-domain-music.md) domain ‖ [13b](M3-13b-contract-music.md) contract ‖ [13f](M3-13f-corpus-music.md) corpus → [13c](M3-13c-ffmpeg-audio-tags-covers.md) tags/covers ‖ [13d](M3-13d-sqlite-music.md) sqlite ‖ [13e](M3-13e-scan-music-pipeline.md) scan → [13g](M3-13g-composition-root-music.md) wiring → [13h](M3-13h-web-music.md) web + E2E | backend, media (13c, 13f), frontend (13h) |
| Audiobooks | [14a](M3-14a-domain-audiobooks.md) ‖ [14b](M3-14b-contract-audiobooks.md) ‖ [14f](M3-14f-corpus-audiobooks.md) → [14c](M3-14c-ffmpeg-chapters.md) ‖ [14d](M3-14d-sqlite-chapters.md) ‖ [14e](M3-14e-scan-audiobook-pipeline.md) → [14g](M3-14g-composition-root-audiobooks.md) → [14h](M3-14h-web-audiobooks.md) | backend, media (14c, 14f), frontend (14h) |
| Photos | [15a](M3-15a-domain-photos.md) ‖ [15b](M3-15b-contract-photos.md) ‖ [15g](M3-15g-corpus-photos.md) → [15c](M3-15c-photometa-exif-reader.md) → [15d](M3-15d-ffmpeg-photo-thumbnails.md) ‖ [15e](M3-15e-sqlite-photos.md) ‖ [15f](M3-15f-scan-photo-pipeline.md) → [15h](M3-15h-composition-root-photos.md) → [15i](M3-15i-web-photos.md) | backend, media (15c, 15d, 15g), frontend (15i) |
| Discs | [16a](M3-16a-domain-discs.md) → [16b](M3-16b-fs-disc-detection.md) ‖ [16c](M3-16c-disc-title-selection.md) ‖ [16e](M3-16e-iso-reader.md) → [16d](M3-16d-disc-encryption-detection.md) ‖ [16f](M3-16f-ffmpeg-disc-input.md) → [16h](M3-16h-scan-playback-discs.md) ‖ [16i](M3-16i-corpus-discs.md) ‖ [16g](M3-16g-ffmpeg-iso-input.md) → [16j](M3-16j-packaging-ffmpeg-capabilities.md) → [16k](M3-16k-composition-root-discs.md) | media, backend (16a, 16h, 16k), synology (16j) |

The four chains run in parallel except for shared files: `api/openapi.yaml` (13b → 14b → 15b,
sequential), migration numbers (13d → 14d → 15e), `restapi` `libraryKinds` and `cmd/mivo` (the
wiring tasks 13g, 14g, 15h, 16k merge one after another), `internal/app/scan/run.go` (one
dispatch line each; rebase). Audiobooks reuse the music tag/cover adapters, so 14 starts after
13c/13e are merged; discs and photos have no cross-dependency and may start first.
Each chain's wiring task enables its kind in `libraryKinds`, so the tile appears in the
add-library dialog. Disc ISO (16e, 16g) may ship one release after folder discs.

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
7. M3-18, then the milestone summary for the owner and the release decision (after lane L of
   the library management, below).

**Lane L – library management (M3-19).** Runs beside the waves above; it starts after the
data model tasks of wave 1 and after ADR 0028 is written. No lane task shares a file with a
wave task running at the same time.

| Lane | Tasks (parallel) | Runs beside | Entry condition |
|---|---|---|---|
| L1 | M3-19a ‖ M3-19b ‖ M3-19c | wave 2 | M3-02, M3-04, M3-11 merged; ADR 0028 written. In wave 2 nobody else edits `api/openapi.yaml`, `restapi`, `platform/config`; M3-19b adds new files to `domain/library` only |
| L2 | M3-19d ‖ M3-19e | waves 3–5 | L1; M3-06 merged (M3-19e adds new files to `app/library`; `service.go` only through an options struct) |
| L3 | M3-19f ‖ M3-19i ‖ M3-19j ‖ M3-19k ‖ M3-19m | wave 7 (M3-18 touches only `e2e/**`) | M3-11b merged (it owns `sqlite`, `app/scan`, `restapi` and `cmd/mivo` until then); M3-12b merged (router) |
| L4 | M3-19g ‖ M3-19h ‖ M3-19n | – | L3. M3-19g and M3-19h own different `queries/*.sql`; if sqlc emits a shared `querier.go`, the second to merge rebases and regenerates |
| L5 | M3-19l – **checkpoint**: libraries are managed through the API, Docker starts empty | – | L4, M3-19c, M3-19d |
| L6 | M3-19o | – | M3-19l, M3-19n, M3-18 |

The milestone summary and the release decision follow M3-18 **and** M3-19o. The second wave
(M3-13 … M3-16) starts after M3-19l, because it touches the same migrations, scan and
contract.

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
| mivo-architect | M3-01 (done), M3-19 (parent, done), ADR 0028 (before lane L1), the split of M3-13 … M3-16, ADR 0027 Notes after deviations |
| mivo-backend | M3-02, M3-03, M3-03b, M3-04, M3-04b, M3-05, M3-05b, M3-05c, M3-05d, M3-06, M3-06b, M3-07, M3-07b, M3-09, M3-09b, M3-10, M3-10b, M3-11, M3-11b, M3-13, M3-14, M3-19a … M3-19l |
| mivo-media | M3-08, M3-17, M3-15, M3-16 (and the probing/cover parts of M3-13, M3-14) |
| mivo-frontend | M3-12, M3-12b, M3-18, M3-19m, M3-19n, M3-19o |
| mivo-synology | none in milestone 3; SPK-17 (wizard without folder question) depends on M3-19 |
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
- **DECIDED 2026-11-04 (owner delegated), ADR 0029: OD-M3-4 = B** (pure-Go EXIF, FFmpeg
  thumbnails, RAW/HEIC via embedded previews or capability probe; no libvips/exiftool) and
  **OD-M3-5 = stock FFmpeg** (pure-Go main-title selection + `concat`, loopback source for ISO;
  no libbluray/libdvdnav/libdvdcss; only SPK/Docker task: M3-16j verification).
- **Scope cut to confirm:** air-date episode names (`Show 2024-01-05`) are not parsed in M3;
  such files become unnumbered episodes (ADR 0027 §4).
- **DECIDED 2026-11-04 (owner delegated): ADR 0028 accepted, OD-M3-6 = A (several folders),
  OD-M3-7 = A (Docker starts empty, `/media` browse root only).** Original text follows.
- **ADR 0028 acceptance** (library management before accounts; residual risks: reverse
  proxies and DNS rebinding until M7, details in M3-19 Notes "Security").
- **OD-M3-6 Several folders per library** (M3-19f, M3-19h, M3-19i): A now, as asked in the
  owner's request (recommended) / B one folder per library in M3, several in M7 (saves M3-19h
  and part of M3-19f/i).
- **OD-M3-7 Docker default** (M3-19l): A replace `MIVO_LIBRARIES=/media` by
  `MIVO_BROWSE_ROOTS=/media` so Docker starts empty like the SPK (recommended) / B keep the
  variable as a one-time seed.

## Approvals the orchestrator needs from the user

- Network: module download `github.com/fsnotify/fsnotify` (M3-11); promotion of
  `golang.org/x/text` (already in the module cache as an indirect dependency; M3-07, M3-07b);
  govulncheck database (all Go tasks).
- Docker: `sqlc/sqlc:1.31.1` for M3-05, M3-05b, M3-05c, M3-05d, M3-11b; the FFmpeg 7.1 image
  check of M3-08; the Linux watcher tests of M3-11.
- Release notes of the next version must announce the one-time catalog rebuild (re-probe of
  every file) – collected by M3-11b – and library management in the app (Docker: add the
  library once in Settings › Libraries) – collected by M3-19l.
- Docker: `sqlc/sqlc:1.31.1` also for M3-19f, M3-19g, M3-19h; the image build and release
  smoke of M3-19l; Linux permission tests of M3-19d.
- A `CLAUDE.md` change, if M3-19l proposes one for the release smoke (only with the owner's
  approval).

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
- Library management (15 slices) adds a second migration (`00004`) to the same release as
  `00003`; users upgrading from 0.1.0-alpha.2 see one catalog rebuild, and configured libraries
  are imported again on the first start (M3-19 Notes "Persistence").
- Management without accounts: the guard admits local, same-origin requests only; reverse
  proxies (DSM's included) and DNS rebinding remain open until M7 and are documented.
