# 0027 Library model v2: hierarchy, local metadata, artwork and scanning (milestone 3)

- Status: Proposed (2026-11-04, deliverable of task M3-01; binding for M3 tasks until the owner
  accepts or rejects it)
- Date: 2026-11-04
- Refines: ADR 0022 (the milestone-2 parts it names as interim: one item per file, title from
  the file name, item kinds `movie|episode`, "renaming creates a new item", watcher and periodic
  scans "arrive in milestone 3"), ADR 0018 (new configuration keys), ADR 0004 (schema
  conventions), plan "Bilder" row (artwork is resized with FFmpeg, not `vipsthumbnail`, see 8)
- Related: ADR 0010 (no outbound connections), ADR 0019 (layer matrix), ADR 0024 (signed URL
  readiness), `docs/design/ux-spec.md` (element ids), `docs/research/plex-walkthrough-2026-11-04.md`

## Context

Milestone 2 lists every video file as one flat item titled after its file name. Milestone 3
("Bibliotheken komplett") must turn files into movies and shows with seasons and episodes,
read local Kodi NFO files and local artwork, serve posters in fixed sizes, group alternative
versions and extras, and keep the catalog current with a watcher and periodic scans. Music,
audiobooks, photos and disc images follow in a second wave of the same milestone.

The owner decided the order 3 → 6 → 5 → 4 → 7 (OD-UX-2 B). Milestone 6 (TMDB, edit dialog with
field locks, artwork from providers, "correct match") comes directly after this one, so the
model must already hold provider IDs, per-field locks, several artwork sources and an ordered
list of metadata sources. Watch state (OD-UX-3) is still open; the model must not depend on
it, but stable item identities make it cheaper whichever way it is decided.

Constraints that stay: library roots are read-only and nothing is written next to the media
(ADR 0022); the domain imports the standard library only (ADR 0019); no outbound connection
is made for local metadata (ADR 0010); the catalog is rebuildable by a rescan because no
user-owned data exists yet.

## Decision

### 1 Library kinds

- Video slice: `movies` and `shows` (unchanged values).
- Second wave: `music`, `audiobooks`, `photos`. Each kind is accepted by the configuration and
  the domain only from the task that implements its scan (M3-13, M3-14, M3-15); the schema
  `CHECK` lists all five now, so the second wave adds tables but never rebuilds `libraries` or
  `items`.
- Disc images are not a library kind: ISO files and `BDMV`/`VIDEO_TS` folders appear in movie
  and show libraries (M3-16).
- "Mixed videos" (plan: `mixed`, UX LS-01) is not part of milestone 3.

### 2 Item hierarchy

| Kind | Parent | Created from |
|---|---|---|
| `movie` | none | one or more video files (versions, parts) |
| `extra` | `movie` or `show` | one video file in an extras folder or with an extras suffix |
| `show` | none | the show folder (or the show name of files directly in the root) |
| `season` | `show` | season folder or the season number of an episode |
| `episode` | `season` | one or more video files (versions) with the same episode number |
| `artist`, `album`, `track`, `audiobook`, `photo_album`, `photo` | (second wave) | reserved in the schema; shapes are fixed by M3-13 … M3-15 |

- Only `movie`, `episode`, `extra` (and the second-wave leaves) own media files. `show` and
  `season` are containers.
- Library listings show top-level items only (movies, shows). Extras are never listed on
  their own; they belong to their parent's detail. A library's item count counts top-level
  items.
- Specials are season 0 and sort after the regular seasons (UX DT-20).

### 3 Item identity and placement

- Every item has a **match key**, computed by the domain from the library-relative path and
  unique per `(library, parent, kind)`. Rescans find existing items by key and keep their IDs
  and creation times, so a changed file, a re-probe, a new version or a renamed episode file
  (`S01E01.mkv` → `S01E01 - Pilot.mkv`) keeps the item.
- Keys: show = normalised show folder (or `name:` + normalised show title for files directly in
  the root); season = season number; episode = `<episode>` or `<episode>-<end>` (multi-episode
  file); folder movie = normalised folder path; loose movie = normalised directory + title +
  year; extra = parent key + normalised relative path.
- **Placement is recomputed on every scan** from the path (a pure parse, no I/O). Probing still
  happens only for new or changed files. A file whose computed placement changed (new parser
  rules, a folder renamed) moves to the new item without a re-probe. Items left without files
  are removed bottom-up (episode → season → show); extras without a parent are not stored.
- Consequence for milestone 7 (and OD-UX-3): renaming a show or movie *folder* creates new
  items. Watch state must be keyed so that it can survive that (for example by provider ID or
  a re-attach step); this ADR does not decide it.
- **Text normalisation at the boundary.** File names and NFO text enter the domain in Unicode
  NFC. The adapters that read them (`libraryfs`, `metadata/nfo`) normalise with
  `golang.org/x/text/unicode/norm`; the walker reports the raw relative path (used to open the
  file and stored in `media_files`) and the NFC path (used for parsing and keys). The domain
  stays standard-library only (ADR 0019) and folds case with `unicode`/`strings`.

### 4 Name parsing rules (pure, `internal/domain/naming`)

Movies (M3-03):
- `Title (Year)`, `Title [Year]`, `Title.Year.<release tags>`; a year is 1880–2100 and the
  last year-like token wins (`Blade Runner 2049 (2017)`, `2001 A Space Odyssey (1968)`,
  `1917 (2019)`). Release tags (resolution, codecs, sources, audio, groups) end the title.
- **Folder movie:** a video file in a directory other than the root belongs to the
  directory's movie when its base name starts with the directory name (normalised) or when it
  is the directory's only video file. Title and year then come from the directory name; the
  rest of the file name is the **version label** (`Movie (2010) - 4K.mkv` → `4K`, Plex style
  `{edition-Director's Cut}` → `Director's Cut`). Other files are **loose movies** parsed from
  their own name.
- **Parts:** `cd1`, `disc2`, `part3`, `pt1` (with a separator) mark parts of one version; each
  part is a file with a part number. Joined playback of parts is not in milestone 3.
- **Extras:** folders `Trailers`, `Featurettes`, `Behind The Scenes`, `Deleted Scenes`,
  `Interviews`, `Scenes`, `Shorts`, `Other`, `Extras` (case-insensitive) and the suffixes
  `-trailer`, `-featurette`, `-behindthescenes`, `-deleted`, `-interview`, `-scene`, `-short`,
  `-other` (and a file named `trailer`). Extra types: `trailer`, `featurette`,
  `behind_the_scenes`, `deleted_scene`, `interview`, `scene`, `short`, `other`. A suffixed
  extra attaches to the movie its base name parses to; a folder extra to the folder movie of
  the parent directory.
- **Samples** (`sample` as the whole name, a `-sample`/`.sample` suffix, or inside a `Sample`
  folder) are ignored.

Shows (M3-03b):
- Show = first directory below the root (year in parentheses becomes the show year); for files
  directly in the root, the text before the episode marker.
- Season = `Season N`, `Staffel N`, `S N`, `Specials` (0) folder, or the season of an episode
  marker.
- Episode markers: `SxxEyy` (season 1–4 digits, episode 1–4 digits), multi-episode
  `S01E01-E02`, `S01E01E02`, `S01E01-02`, `1x02`, `1x02-1x03`, `E02`/`Ep02`/`Episode 2` (season
  from the folder, else 1).
- **Absolute numbering (UX DT-24, regression of the Plex walkthrough):** a bare number after the
  show title (`Show - 001`, `Show 130`, `[Group] Show - 130 [1080p] [ABCD1234]`) is
  an **episode number**, stored also as the absolute number (an explicit marker such as `E130` is a plain episode number and sets no absolute number). It is placed in season 1, or in the
  season of its season folder. A three- or four-digit number is **never** split into season and
  episode (`105` is episode 105, not S1E05). Resolutions, codec and audio tags, CRC checksums in
  brackets, release groups, `v2` revision suffixes and the show year are never episode or
  season numbers. A show of 131 files `Dragon Ball Super - 001` … `- 131` is one show, one season,
  131 episodes.
- Air-date episode names (`Show 2024-01-05`) are not parsed in milestone 3: such files become
  episodes without a number in their folder's season (or season 1), sorted by title.
- Extras folders inside a show or season folder attach to the show.

### 5 Data model (goose migration `00003`)

The migration **drops and recreates the catalog** (`libraries`, `items`, `media_files`,
`media_streams`) and adds the new tables; the startup sync recreates the libraries and the
next scan re-probes every file (OD-M3-1). All tables `STRICT`, ADR 0004 conventions; dates
without time are `TEXT` `YYYY-MM-DD` with a `GLOB` check.

- `libraries`: kind `CHECK IN ('movies','shows','music','audiobooks','photos')`.
- `items`: adds `match_key` (unique with `library_id`, `ifnull(parent_id,'')`, `kind`),
  `original_title`, `year`, `release_date` (release or air date), `summary`, `tagline`,
  `content_rating` (raw text such as `FSK 12` or `PG-13`; normalisation by country is
  milestone 7), `number` (season, episode or track number), `number_end` (multi-episode),
  `absolute_number`, `disc_number` (music), `extra_type`, `metadata_language` (per-item
  override, UX ED-07, empty until M6). Kind `CHECK` lists all kinds of section 2.
- `media_files`: adds `version_label`, `part_number`, `layout` (`file|iso|bdmv|video_ts`, only
  `file` until M3-16). The primary file is the first by `(part_number, relative_path)`.
- `media_streams`: unchanged.
- `tags(id, kind, name)` with kinds `genre|studio|country|collection` (`collection` is filled
  from NFO `<set>` and used in milestone 7) and `item_tags(item_id, tag_id, position)`.
- `people(id, name, sort_name)` and `item_people(item_id, person_id, role, character,
  position)`, role `actor|director|writer|producer|guest_star` now (`author`, `narrator` with
  M3-14). People are identified by name in milestone 3; milestone 6 adds
  `person_provider_ids` and portraits.
- `item_provider_ids(item_id, provider, external_id)`, providers `tmdb|imdb|tvdb` now
  (`musicbrainz_*` with M3-13). Filled from NFO in milestone 3, used for matching in M6.
- `CHECK` constraints (ADR 0022 Notes M2-10 rule): sets that are complete now are closed in
  the schema (library kinds, item kinds, extra types, artwork sources, tag kinds, sidecar
  kinds, media file layouts); sets that grow in later tasks (person roles, providers, artwork
  kinds, lockable field names) are only checked for non-empty text and validated by the
  domain parsers in the adapter, so adding a value never rebuilds a table.
- `item_field_locks(item_id, field)`: the lockable fields of section 6. Milestone 3 never sets
  a lock but every write respects them, so the M6 edit dialog (UX ED-02) only adds the UI and
  the writes.
- `artwork(id, item_id, kind, source, library_id, relative_path, media_file_id, stream_index,
  data_path, provider, remote_url, size_bytes, modified_at, width, height, tag, selected,
  created_at)` with exactly one source-specific column group per source and at most one
  `selected` row per `(item_id, kind)` (partial unique index).
- `library_sidecars(library_id, relative_path, kind, size_bytes, modified_at)`: NFO and image
  files seen by the last scan, for change detection like `media_files`.

### 6 Metadata sources, merge and field locks (`internal/domain/metadata`)

- A **metadata record** holds: title, sort title, original title, year, release date, summary,
  tagline, content rating, genres, studios, countries, collections, people (name, role,
  character, order) and provider IDs.
- **Sources** in milestone 3, in priority order: `nfo`, then `filename` (the parser). Embedded
  container tags of videos are not used (release names in MKV titles are noise). Milestone 6
  inserts `tmdb` and makes the order configurable per library kind (UX AD-22); the merge
  already takes an ordered list.
- **Merge** is a pure domain function: per scalar field the first source with a value wins;
  a list field (genres, studios, countries, collections, people, provider IDs) is taken whole
  from the first source that has any value; a **locked field keeps its stored value** whatever
  the sources say.
- Lockable fields: `title`, `sort_title`, `original_title`, `year`, `release_date`, `summary`,
  `tagline`, `content_rating`, `genres`, `studios`, `countries`, `collections`, `people`,
  `provider_ids`, `artwork.poster`, `artwork.backdrop`, `artwork.thumb`, `artwork.logo`,
  `artwork.banner`.
- The catalog writer additionally refuses to overwrite a locked field (integrity guard), so a
  scan that read the locks before an edit in M6 cannot undo it.
- Placement (season and episode numbers, item kind) comes from the path only. NFO numbers do
  not move items in milestone 3; provider episode ordering (absolute/DVD) is milestone 6.
- **Sort title:** NFO `<sorttitle>` or the title without one leading English or German article
  (`The`, `A`, `An`, `Der`, `Die`, `Das`, `Ein`, `Eine`), case-folded (UX LB-22). Articles per
  metadata language come with M6.

### 7 Kodi NFO (`internal/adapters/metadata/nfo`)

- Files: `<video base name>.nfo` and `movie.nfo` (folder movie only) for movies,
  `tvshow.nfo` in the show folder, `season.nfo` in a season folder, `<video base name>.nfo`
  for episodes (a file with several `<episodedetails>` roots describes a multi-episode file).
- Read through the library's `os.Root` with a size limit of 1 MiB; UTF-8 (BOM allowed) and
  ISO-8859-1 declarations are accepted, anything else is logged and the file ignored. Text is
  NFC-normalised and trimmed; unknown elements are ignored; a malformed file never fails a
  scan.
- Elements read: `title`, `originaltitle`, `sorttitle`, `year`, `premiered`, `releasedate`,
  `aired`, `plot` (else `outline`), `tagline`, `mpaa`/`certification`, `genre`, `studio`,
  `country`, `set/name` (or text `set`), `director`, `credits` (writers), `actor`
  (`name`, `role`, `order`), `uniqueid` (`type`, `default`), `id`, `tmdbid`, `imdbid`,
  `tvdbid`. Provider IDs are validated (`tmdb`/`tvdb` digits, `imdb` `tt` + digits).
- "URL NFO" files (only a TMDB/IMDb/TVDB URL) and XML followed by such a URL yield provider IDs.
- **Nothing referenced by an NFO is fetched** (actor `thumb`, `fanart` URLs): ADR 0010.

### 8 Local artwork and the image endpoint

- Artwork kinds: `poster`, `backdrop`, `thumb` (16:9 episode still or movie landscape),
  `logo`, `banner`. `square` is added with music (M3-13).
- Local file names (extensions `jpg jpeg png webp tbn`), in priority order:
  - movie (folder movie): `poster`, `folder`, `cover`, `movie`, `<base>-poster`, `<base>`
    → poster; `fanart`, `backdrop`, `background`, `art`, `<base>-fanart` → backdrop;
    `landscape`, `thumb`, `<base>-thumb` → thumb; `clearlogo`, `logo` → logo; `banner` → banner.
    For a loose movie only the `<base>…` names apply.
  - show folder: `poster`, `folder` → poster; `fanart`, `backdrop` → backdrop; `clearlogo`,
    `logo` → logo; `banner`; `seasonNN-poster`, `season-specials-poster` → season posters.
  - season folder: `poster`, `folder` → season poster.
  - episode: `<base>-thumb`, `<base>` → thumb.
- **Embedded** artwork: attached-picture streams (MP4 `covr`, Matroska `cover*` attachments,
  which FFmpeg exposes as attached pictures) are poster candidates after local files.
- **Selection:** a locked artwork kind keeps its selection; otherwise local by priority, then
  embedded. Milestone 6 adds `provider` and `upload` sources (files in the **data
  directory**, never in the library) with their position in the ordered source list.
- **Fallback in the API:** a season without a poster shows its show's poster; episodes and
  extras without a thumb have none (the client shows a placeholder; generated stills are M4,
  UX DT-23).
- **Rendering with FFmpeg** (OD-M3-2): FFmpeg is already bundled on every target, runs as a
  separate process (memory of a large decode never stays in the server, ADR 0015) and decodes
  JPEG, PNG, WebP and attached pictures. Arguments keep `-protocol_whitelist file` and the
  input `-format_whitelist` of ADR 0022 (image2 only for paths without `%`), and set a decoder
  `-max_pixels` limit. libvips (`vipsthumbnail`, plan) stays a candidate for photos (HEIC/RAW),
  decided in M3-15.
- **Endpoint (UX LB-44):** `GET /api/v1/images/{imageId}?width=<w>&tag=<t>`, a plain handler
  outside the strict server (like the stream routes). Widths are a fixed set
  `160 320 480 640 960 1280 1920`; anything else is 400. Never upscaled: a narrower source is
  served at its own width. Output JPEG (quality constant) for every kind except `logo`, which
  is PNG to keep transparency.
- **Tag and caching:** `tag` = the first 16 hex digits of SHA-256 over the source identity,
  size, modification time and a render version constant. API responses carry `{id, tag}`; a
  request whose tag matches gets `Cache-Control: private, max-age=31536000, immutable`, any
  other request `private, no-cache`; both carry `ETag: "<tag>-<width>"` and answer
  `If-None-Match` with 304. `private` because milestone 7 puts images behind authentication
  (`<img>` cannot send headers: cookie or signed URL, ADR 0024 readiness).
- **Cache:** rendered files in `<cache>/images/<id[0:2]>/<id>/<tag>-<width>.<ext>`, written to
  a temporary file and renamed. Stale tags and unknown IDs are purged at startup and after each
  finished scan. No size cap in milestone 3 (cache cleanup UI is AD-41, milestone 7).
- **Limits:** source file ≤ 32 MiB, ≤ 64 megapixels, render timeout 15 s, at most two renders
  at a time server-wide, concurrent requests for the same file render once.

### 9 Scanning v2

- The walker reports video files and sidecar files (`nfo`, images) with their class; the
  classification by extension is a domain policy. NAS system folders stay skipped (ADR 0022).
- One scan: walk → recompute placements of all video files → diff files and sidecars with the
  catalog → remove vanished files (only after a complete walk, unchanged ADR 0022 rules) →
  probe new and changed files → save placements → for every item whose files or sidecars
  changed (or that is new): read its NFO, merge, save metadata; collect artwork candidates,
  select, save. Unchanged items are not touched.
- **Watcher** (`fsnotify`): one recursive watch per available library root; events are
  debounced (quiet period 30 s, at most 5 min after the first event) and trigger a normal
  scan of that library; an event during a running scan schedules one more scan after it.
  When watches cannot be added (inotify limit, network file systems that send no events) the
  server logs one warning per library and relies on the periodic scan.
- **Periodic scan:** every library is scanned again after the configured interval, measured
  from the end of its last scan. The startup scan stays (ADR 0022 OD-3).
- Configuration keys (ADR 0018 rules):

  | YAML key | Environment | Default | Validation |
  |---|---|---|---|
  | `scan.watch` | `<PREFIX>_SCAN_WATCH` | `true` | `true`/`false`, case-insensitive (like `server.lan_only`) |
  | `scan.interval` | `<PREFIX>_SCAN_INTERVAL` | `12h` | Go duration; `0` disables; else 15m–168h |

### 10 Second wave (decided in their tasks)

- **Music (M3-13):** `artist → album → track` from embedded tags (ffprobe format tags:
  `album_artist`, `artist`, `album`, `track`, `disc`, `date`, `genre`, MusicBrainz IDs), folder
  fallback, embedded and `cover`/`folder` artwork, artwork kind `square`.
- **Audiobooks (M3-14):** one `audiobook` item per folder or per M4B file; chapters from the
  file's chapters (M4B) or from part files; author and narrator as people.
- **Photos (M3-15):** folders as `photo_album`, `photo` items with EXIF date, camera and
  location; tool choice (OD-M3-4).
- **Disc images (M3-16):** ISO, `BDMV` and `VIDEO_TS` as one media file with a `layout`;
  needs FFmpeg with libbluray/libdvdnav and a demuxer whitelist decision (OD-M3-5); encrypted
  discs are refused (plan).

## Consequences

- Upgrading from 0.1.0-alpha.2 drops the catalog once; the first scan re-probes everything
  (minutes on a NAS for thousands of files). Release notes must say so.
- Item IDs become stable across rescans and file changes, which M6 (locks, provider IDs) and
  M7 (watch state) rely on; folder renames still create new items.
- Parser fixes take effect on the next scan without a re-probe.
- The image cache grows with the number of artworks × widths requested; it is safe to delete
  at any time.
- A new production dependency `github.com/fsnotify/fsnotify` (watcher) and the promotion of
  `golang.org/x/text` from indirect to direct (NFC normalisation in adapters).
- The second-wave kinds need their own contract, config and scan work; their tables are added
  by later migrations, the core tables are not rebuilt again.

## Open decisions for the owner

Tasks follow the recommendation until the owner answers.

- **OD-M3-1 Upgrade path.** A: drop and recreate the catalog, re-probe once (**recommended**:
  no user data exists, simplest and safest migration). B: migrate files and streams in place
  and regroup them (no re-probe, but a table rebuild with foreign keys switched off).
- **OD-M3-2 Artwork resizing.** A: FFmpeg (**recommended**: already bundled, no new native
  dependency in Docker and the SPK). B: libvips now (plan wording; faster, but a new bundled
  library on three architectures before photos need it).
- **OD-M3-3 Scan defaults.** Watcher on and periodic scan every 12 h (**recommended**: few
  disk wake-ups on a NAS with hibernation), or a shorter interval (6 h, 1 h).
- **OD-M3-4 Photo tooling (before M3-15).** A: bundle `exiftool` and libvips/libheif (plan;
  complete RAW/HEIC support, larger image and SPK). B: pure-Go EXIF reader plus FFmpeg for
  JPEG/PNG/WebP/HEIF, RAW via embedded previews only.
- **OD-M3-5 Disc images (before M3-16).** Require libbluray and libdvdnav in every bundled
  FFmpeg build (Docker image, SPK static build SPK-15/16) and whitelist the `bluray`
  protocol/`dvdvideo` demuxer for directory and ISO inputs, or limit disc support to
  platforms whose FFmpeg has them.
