# 0022 Media library model and scanning (milestone 2)

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-30
- Refines: "Datenmodell (Kern)" in `docs/plan.md`, ADR 0004 (schema conventions), ADR 0018
  (new configuration keys)

## Context

Milestone 2 is the first slice that touches user files: scan a folder, probe every video
with ffprobe, list the result in the web UI. The server still has no login (accounts arrive in
milestone 7) and runs on the local network. In Docker, `/media` is mounted read-only. NAS shares
contain system folders (`@eaDir`, `#recycle`) and may be mounted after the server starts.
Name parsing (movies/shows), watchers, NFO and artwork arrive in milestone 3.

## Decision

**Library definitions (OD-1 A, decided by the project owner on 2026-10-30)**
- In milestone 2, libraries are defined in the configuration, not through the API:
  YAML key `libraries` is a list of `{name, path, kind}`. The environment variable
  `<PREFIX>_LIBRARIES` holds an OS path list (`filepath.SplitList`: `;` on Windows, `:` on
  Linux); each entry becomes a library named after its last path element with kind `movies`.
  When the variable is set it replaces the YAML list (ADR 0018 precedence).
- Relative paths resolve against the config directory (ADR 0018). After resolution every root
  must be absolute, unique and not nested in another root. A root must not contain or lie
  inside the config, data, cache or transcode directory. Comparisons are case-insensitive on
  Windows. A root that does not exist is **not** a startup error (NAS shares mount late); the
  library is reported as unavailable and its scan fails with `root_unavailable`.
- At startup the definitions are synced into the database. A library's identity is its cleaned
  absolute root path: a new path creates a library with a new UUID, a removed path deletes the
  library and, by cascade, its items. Nothing user-owned exists yet in milestone 2; milestone 7
  (watch state) must revisit deletion.
- The REST API exposes libraries read-only. Creating libraries from the UI comes with the
  setup wizard in milestone 7, after authentication exists: an unauthenticated "add library"
  endpoint would let any device on the network expose any server directory.
- Library kinds are `movies` (default) and `shows`. Milestone 2 lists every item of a library
  flat, regardless of kind (OD-2). Milestone 3 adds the hierarchy for shows.

**Data model** (goose migration `00002`, all tables `STRICT`, ADR 0004 conventions)
- `libraries(id, name, kind, root_path UNIQUE, created_at, updated_at)`
- `items(id, library_id → libraries ON DELETE CASCADE, parent_id → items NULL (reserved for
  milestone 3), kind, title, sort_title, created_at, updated_at)`
- `media_files(id, item_id → items CASCADE, library_id → libraries CASCADE, relative_path,
  size_bytes, modified_at, container, format_name, duration_ms, bit_rate, probed_at,
  probe_error, UNIQUE(library_id, relative_path))`
- `media_streams(media_file_id → media_files CASCADE, stream_index, kind, codec, profile,
  level, pixel_format, bit_depth, width, height, frame_rate, color_primaries,
  color_transfer, color_space, hdr, channels, channel_layout, sample_rate, language, title,
  is_default, is_forced, is_attached_picture, bit_rate, PRIMARY KEY(media_file_id,
  stream_index))`
- Paths are stored **relative to the library root**, with `/` as separator, never absolute.
  The absolute path is computed when a file is opened, so a moved mount point only needs a
  config change.
- One item per video file in milestone 2. The title is the file name without extension, with
  `.` and `_` turned into spaces and trimmed; the sort title is its case-folded form.
  The name parser of milestone 3 replaces this.

**Scanning**
- The walker opens each library root with `os.Root` and walks inside it, so no `..` or symlink
  can leave the root. Symlinks that resolve inside the root are followed; symlinks that
  escape it are skipped and logged at info level (OD-8).
- Only files with a video extension are considered: `mkv mk3d mp4 m4v mov avi divx ts m2ts mts
  webm flv wmv asf ogv 3gp 3g2 mpg mpeg vob rm rmvb mxf` (case-insensitive). Hidden entries
  (leading `.`) and NAS/OS system folders are skipped: `@eaDir`, `#recycle`, `#snapshot`,
  `.@__thumb`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`.
- Change detection uses `(relative path, size, modification time in ms)`. Unchanged files are
  not probed again. New or changed files are probed with ffprobe with a bounded concurrency of
  two (NAS disks) and a per-file timeout. A probe failure does not fail the scan: the file is
  stored with a `probe_error` code and shown as unavailable. Files that disappeared are deleted.
  A file without a video stream is skipped (it is not a video).
- One scan per library at a time. Scans run in the background, are owned by the scan use case,
  stop when the server shuts down, and keep their status (state, counts, last error code) in
  memory only. Triggers in milestone 2: the API (button in the UI) and server startup (OD-3).
  Watcher and periodic scans arrive in milestone 3.
- The server never writes into a library root.

**Path safety**
- The API never accepts file system paths. Files are addressed by ID only.
- Every open of a media file goes through `os.Root` of its library root. A path handed to
  FFmpeg is resolved (symlinks evaluated) and verified to lie inside the evaluated root right
  before the process starts, and is passed as `file:<absolute path>` so FFmpeg never
  interprets a file name as a protocol or option. The small time-of-check/time-of-use window
  between verification and FFmpeg opening the file is accepted: FFmpeg cannot take an open
  file handle portably, and only someone with write access inside the library could exploit it.

**Configuration keys added in milestone 2** (multi-word keys use snake_case, so the environment
name stays unambiguous: `transcode.max_sessions` → `<PREFIX>_TRANSCODE_MAX_SESSIONS`)
- `libraries` (list, see above), `tools.ffmpeg` and `tools.ffprobe` (default `ffmpeg` and
  `ffprobe`; a bare name is looked up on `PATH`, anything with a path separator follows the
  ADR 0018 path rules), `transcode.max_sessions` (ADR 0024, default 2 per OD-4, range 1–8).

## Consequences

- Docker users get a working library by mounting `/media` (OD-1: the Docker default is
  accepted) without touching the UI.
- Library management in the UI waits for milestone 7. Until then, changes need a config edit
  and a restart.
- Renaming or moving a file creates a new item; nothing is lost because milestone 2 stores no
  user data.
- `os.Root` requires Go 1.24+; the module uses 1.27.

## Notes (M2-12 r2)

Implementation details of the walker in `internal/adapters/fs/libraryfs` that the scan (M2-19)
relies on. They refine the scanning section; the decision itself is unchanged.

- **One listing per physical directory.** The walk lists the real directory tree first (no
  links followed), in name order, and follows directory links afterwards in discovery order.
  Every physical directory is listed once, so a file is reported under its path without links,
  and loops or cross-linked folders ("by genre", "by year") cannot multiply the walk. A
  directory reached again through a link is skipped and logged at debug level.
- **File links** are aliases the user made and are reported under their own path. One file can
  therefore appear in the catalog under several relative paths; the scan must expect such
  duplicates.
- **Refused links.** `os.Root` refuses links with an absolute target, even when the target lies
  inside the root, and therefore every Windows junction (OD-8 A). They are skipped and logged at
  info level.
- **Skipped paths.** `Walk(ctx, root, visit) ([]string, error)` returns the root-relative paths
  it could not read: unreadable directories and entries, directories below the depth limit and
  real-tree directories with a colliding identity (below). The scan keeps catalog entries equal
  to or below these paths. When `Walk` returns an error the result is nil and the scan aborts
  without deleting anything; a root that is unreadable at the start or becomes unreadable during
  the walk (a dropped NAS share) returns `ErrUnavailable`.
- **Depth limit.** Directories deeper than 64 levels below the root are not listed; they are
  logged at warn level and reported as skipped.
- **Directory identity.** Unix uses device and inode. Windows uses the volume serial number with
  the 128-bit file id (`FileIdInfo`), because ReFS (including Dev Drives) does not guarantee a
  unique 64-bit file index; the 64-bit index is the fallback where `FileIdInfo` is not
  supported. An all-zero or invalid id is unknown: it is never recorded and such a directory is
  always listed (the depth limit still bounds the walk). Two directories of the real tree with
  the same id can only mean the file system reports non-unique ids, so the second one is logged
  at warn level and reported as skipped instead of being dropped silently.

## Notes (M2-10)

Additions from the schema migration (`00002_media_catalog.sql`, M2-10); they refine the data
model above without changing the decision.

- `media_streams.codec_tag` (nullable `TEXT`) stores the container FourCC (`media.Stream.CodecTag`,
  ffprobe `codec_tag_string`, e.g. `hvc1` vs `hev1`), which later remux decisions need. `NULL`
  means the container stores no tag; an empty string is rejected.
- `media_streams.frame_rate` is `TEXT` holding the exact fraction (`24000/1001`, `''` when
  unknown) as written by `media.FrameRate.String()` and read by `media.ParseFrameRate`.
- `media_files.probe_error` is `NULL` for a successfully probed file; a failed probe stores its
  code with `container = 'unknown'` and no streams.
- Closed sets have `CHECK` constraints: library kinds `movies|shows`, item kinds
  `movie|episode` (`library.ItemKind`), stream kinds `video|audio|subtitle|other`. Sets that
  grow with later milestones (containers, HDR formats, probe error codes) are only checked for
  non-empty text, because SQLite cannot change a `CHECK` without rebuilding the table; the
  adapter validates them with the domain parsers.
- Every foreign key column leads an index (`items(parent_id)` added), so a cascading delete
  never scans a whole child table per parent row.
- In milestone 2 an item's kind follows from its library's kind (`library.ItemKindForLibrary`):
  when a library's kind changes in the configuration, the startup sync updates its items too.

## Notes (M2-10 review, orchestrator)

- The root path is a library's identity in milestone 2. Changing a root in the config therefore
  removes the old library (cascade) and creates a new one with a new id and a full re-probe; the
  catalog does not survive a moved mount point yet. A stable library identity in the config is
  revisited with milestone 7.
- `items.kind` has a closed `CHECK (kind IN ('movie','episode'))`. Milestone 3 adds `show` and
  `season`; since the catalog is rebuildable by a rescan, the migration may drop and recreate the
  catalog tables instead of rebuilding them in place.
- Path safety for FFmpeg (M2-11 review): `file:` only stops the *name* from selecting a protocol;
  the *content* can still select demuxers that open further URLs (DASH, HLS, concat). Every
  ffprobe/ffmpeg invocation therefore passes `-protocol_whitelist file` before its input (nested
  opens inherit it), so a hostile media file cannot make the server send network requests.
  Producers in M2-17/M2-24 must keep this.
- Symlinks (M2-12): `os.Root` refuses every absolute symlink target and every Windows junction,
  even when the target lies inside the library root. Walk, Resolve and Open therefore follow only
  relative links that stay inside the root; absolute links and junctions are skipped and logged
  (OD-8). NAS users with absolute links should use relative links or add the target folder as its
  own library.

## Notes (M2-18)

- Startup sync: the library use case validates every definition and rejects a root configured
  twice before the catalog is touched (`invalid`, naming the definition's index). Every library
  the sync removes, with its catalog, is logged at warn level with its id, name, root and item
  count, so a typo in the configuration or an empty library list leaves a trace.
- Availability: listing libraries (and the library detail) checks every root at the same time
  with a default timeout of 1 s; a root that does not answer in time is reported as
  unavailable, so a hung NAS mount cannot block the library list.
- A stat on a hung network mount may ignore cancellation. A check therefore runs in its own
  goroutine, and while one runs for a root, later requests join it instead of starting another:
  at most one blocked goroutine per root, however often clients poll. Trade-off: as long as a
  stuck check does not return, its root stays unavailable. No fresh check is started next to
  it, because on a hung mount it would block as well; a mount that recovers normally lets the
  stuck call return too, and the next request checks again.
- Error contract of the catalog reads: `invalid` from the library store or the item reader
  means only a bad client value (the page cursor). A stored row the domain cannot read is a
  server fault; the use case reports it without `invalid`, so the API answers 500, not 400.

## Notes (M2-19)

Additions from the scan use case (`internal/app/scan`, M2-19); they refine "Scanning" above.

- The probe limit of two applies to the whole scan service, not to each scan: all scans share
  one semaphore, so `StartAll` over several libraries on one NAS still runs at most two
  ffprobe processes.
- Catalog entries are deleted only after a complete walk. A root that is missing or
  lost during the walk fails the scan with `root_unavailable` and deletes nothing. A root lost
  later, while resolving a file, stops the scan but keeps the removals already justified by the
  completed walk. Catalog entries at or below a path the walker could not read are kept.
- A file that cannot be resolved (vanished, or a link leaving the root) or probed is stored
  with `probe_failed` (`probe_timed_out` for a timeout) and no streams, so it is listed as
  unavailable and a changed file never keeps its old, playable description.
- Files without a video stream are remembered in memory (path, size, modification time) and
  counted as skipped without a new probe while they stay unchanged; after a restart they are
  probed once more.
- Scan error codes: `root_unavailable` (library folder missing or unreadable),
  `prober_unavailable` (ffprobe cannot run; no file is marked failed), `catalog_failed`
  (catalog read or write failed), `scan_canceled` (stopped by server shutdown; nothing is
  recorded as failed), `scan_failed` (anything else). The finish log line carries the code and
  a cause without absolute paths.
- `finishedAt` is empty while a scan runs; counts, error code and times always describe the
  same scan.

## Notes (M2-24)

- On Linux the HLS producer opens the input through the library's `os.Root` and hands it to
  FFmpeg as an inherited descriptor (`-i file:/proc/self/fd/3`). This closes the window between
  the path check and its use for the producer only; on Windows the producer still passes the
  resolved path, and the ffprobe keyframe lookup still opens the resolved path on every system,
  so the accepted window of the Decision remains there.
- Through the descriptor FFmpeg sees no file extension and detects the container from the content
  alone, while the scan probe also sees the extension; for files with weak signatures the two can
  disagree. `-protocol_whitelist file` keeps FFmpeg off the network but still lets playlist-like
  demuxers (hls, concat) open other local files; pinning the input demuxer is task M2-24b.

## Notes (M2-24b)

- **Input demuxers are whitelisted, not pinned.** Every ffprobe and ffmpeg invocation of
  `adapters/ffmpeg` (scan probe, keyframe lookup, HLS producer) passes
  `-format_whitelist <list>` before its input, next to `-protocol_whitelist file`. The list
  holds only demuxers that read exactly the one file they are given: the video containers
  `media.IdentifyContainer` and the scanner's extensions map to (`matroska`, `mov`, `mpegts`,
  `mpeg`, `mpegvideo`, `avi`, `flv`, `live_flv`, `ogg`, `asf`, `mxf`, `rm`), audio containers
  (`flac`, `mp3`, `wav`, `aiff`, `wv`, `ape`, `dsf`, `iff`, `aac`, `ac3`, `eac3`, `dts`),
  single images (`gif`, `png_pipe`, `jpeg_pipe`, `tiff_pipe`, `webp_pipe`) and subtitle files
  (`srt`, `webvtt`, `ass`, `sup`). Not on it, because they open files named by the content or
  derived from the path: `concat` (ffconcat lists), `hls`, `dash`, `imf`, `vobsub`; also every
  script, device and network demuxer. FFmpeg 7.1.5 and 9.0.2 both check the whitelist right
  after probing and before the demuxer reads its header (where concat and hls open their
  entries) and copy it into nested opens. A file whose content selects another demuxer fails
  with "Format not on whitelist"; it is never handed to the next-best demuxer. Verified with a
  canary file: an unrestricted ffprobe reads it through an ffconcat list named `evil.mkv` and
  through an `.m3u8` playlist, the protected probe, keyframe lookup and producer are refused.
  FFmpeg itself already refuses HLS playlists without an `.m3u8`/`.m3u` extension, but
  detects ffconcat lists by content, also through the producer's `/proc/self/fd/3`.
- **image2** reads image sequences (`frame%d.jpg`) and, in FFmpeg 7.1, `%`-escaped globs; both
  need a `%` in the path. It is allowed only for paths without `%`, where it reads exactly the
  named file. It is the only demuxer FFmpeg opens JPEG files with, so the corpus stays fully
  probeable. A path with `%` (`100% Wolf.mkv`) loses nothing but image2.
- **Why not `-f` pinned from the probe result:** the scan probe has nothing to pin and needs
  the whitelist anyway; `-f` restricts only the top-level demuxer, while the whitelist also
  covers nested opens; pinning would make the probed demuxer travel through the catalog, the
  playback job and the `KeyframeAtOrBefore` port. Pinning would also remove the
  path-versus-descriptor detection difference noted under M2-24, which the whitelist does not:
  both sides are limited to single-file demuxers but may still pick different ones for a file
  with a weak signature. Revisit when the catalog stores the probed demuxer.
- Support for further file kinds (music and photo libraries) extends the list deliberately; a
  demuxer that opens other files needs its own decision first.
