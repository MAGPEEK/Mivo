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
