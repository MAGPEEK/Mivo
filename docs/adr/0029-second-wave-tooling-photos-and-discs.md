# 0029 Second-wave tooling: photos and disc images (OD-M3-4, OD-M3-5)

Status: Accepted (owner delegated all decisions, 2026-11-04)

## Context

ADR 0027 §10 left two decisions open: the photo tooling (OD-M3-4) and disc support (OD-M3-5).
The bundled FFmpeg is already shipped in the Docker image and in the Synology SPK on three
architectures. Every extra native library or tool adds image size, a per-architecture static
build in SPK-15/16, and licence duties (Mivo is GPL-3.0).

## Decision

**OD-M3-4 = B (no libvips, no exiftool).**
- EXIF/GPS/orientation: a small pure-Go parser in `internal/adapters/photometa` (TIFF IFDs in
  JPEG, TIFF-based RAW and ISOBMFF `Exif` items). No new Go dependency.
- Thumbnails: the bundled FFmpeg (same renderer and cache as artwork, OD-M3-2), on demand,
  with orientation applied.
- RAW: the embedded JPEG preview, extracted in pure Go, then rendered by FFmpeg. No RAW
  demosaicing.
- HEIC/AVIF: used when the startup capability probe shows a decoder, otherwise the container's
  embedded preview, otherwise the photo shows an `unsupported` thumbnail state; the scan never
  fails.
- Why: exiftool needs Perl (about 30 MB+ and a per-arch runtime on DSM), libvips/libheif adds
  native libraries (libheif/x265 patent and GPL questions) to three static builds, and the
  EXIF fields the UI needs are a small, stable subset.

**OD-M3-5 = stock FFmpeg, no libbluray/libdvdnav in this milestone.**
- Folder discs (`VIDEO_TS`, `BDMV`): Mivo picks the main title in pure Go (MPLS playlist
  parser; largest VTS set) and feeds the ordered VOB/m2ts clips to FFmpeg through the `concat`
  demuxer and its `mpeg`/`mpegts` demuxers. These are in every default FFmpeg build.
- ISO: a pure-Go UDF/ISO 9660 reader locates the clips as byte extents of the image, and
  FFmpeg reads those ranges with its `subfile` protocol (amended in M3-16g: originally a
  per-session loopback HTTP range source, dropped because the bundled NAS FFmpeg is built
  without network protocols; see Notes). ISO may ship one release after folder support.
- Whitelist additions are exactly: `concat` format, `file` protocol for the list file, and
  `subfile` (plus the `concat` protocol for DVD) for image ranges. No network protocol is
  ever allowed (extends the M2-24b whitelist).
- Encrypted discs (CSS, AACS) are detected heuristically and shown unavailable
  (`disc_encrypted`); no libdvdcss, no keys, no decryption.
- Why: libdvdnav/libdvdread are GPL and libbluray LGPL; they would force custom static FFmpeg
  rebuilds for all architectures and bring in menu navigation we do not need (menus and
  multiple titles are out of scope). Revisit via a new ADR if users need DVD menus or
  seamless-branching Blu-rays; the recipe task would then be a SPK/Docker FFmpeg rebuild with
  `--enable-libbluray --enable-libdvdnav --enable-libdvdread --enable-gpl`.
- Packaging impact: only M3-16j (verification of demuxers/protocols/decoders in both builds).

## Consequences

- No new bundled binaries; Docker and SPK sizes stay unchanged.
- Some Blu-rays (obfuscated playlists, multi-angle) may select the wrong title; limits are
  documented, tests cover the common cases.
- HEIC/RAW quality depends on previews and the FFmpeg build; unsupported files are listed with
  a clear state.
- A new pure-Go parser surface (EXIF, MPLS, IFO, UDF) is fuzz-tested.
- Supersedes the "libvips/exiftool" wording for photos in the plan; ADR 0027 is not rewritten.

## Notes

- Folder discs are change-detected by their index file (`index.bdmv`, `VIDEO_TS.IFO`) only:
  size and modification time of the clips are not part of the scan state (M3-16b).
- Main-title rules (M3-16c, `internal/adapters/disc`): Blu-ray = longest MPLS playlist that
  repeats no clip and whose STREAM clips all exist (ties: lowest playlist number; multi-angle
  items use angle 1; only the in/out span counts toward the duration, and since M3-16g the
  clip is cut to it, see below). DVD = title set with the largest total size of
  `VTS_nn_k.VOB` (k ≥ 1, no gaps); duration = longest PGC in `VTS_nn_0.IFO` (or `.BUP`), unknown (0) when neither parses.
  Playlists/sets with missing clips are skipped; none left → `disc_no_title`. Directory
  listings, file sizes and the total bytes read per disc are capped (hostile input →
  `disc_unsupported`); 0-byte stream clips count as missing; corrupt ISO structures are
  `disc_unsupported`, cancellation is not. AVCHD nesting (`PRIVATE/AVCHD/BDMV`) is found only
  when the walker reports that BDMV folder itself, not inside ISO images.
- Photo thumbnails (M3-15d, `internal/adapters/ffmpeg` `PhotoRenderer`): JPEG, PNG, WebP,
  GIF and TIFF are decoded directly with `-noautorotate` and the EXIF orientation applied as
  explicit `transpose`/`hflip`/`vflip` filters before scaling, so the result does not depend
  on what an FFmpeg version does with EXIF. HEIC/AVIF are decoded directly only when the
  startup probe (`-decoders`, `-demuxers`) lists the `mov` demuxer and `hevc` (HEIC) or
  `libdav1d`/`libaom-av1` (AVIF; FFmpeg's own `av1` decoder needs hardware); FFmpeg then
  applies the container's `irot`/`imir` itself and picks the (grid) picture without `-map`.
  RAW files, undecodable HEIC/AVIF, files over 256 MiB and every direct decode that fails as
  unsupported fall back to the embedded JPEG preview piped to FFmpeg (`jpeg_pipe`, protocol
  `pipe` only); without a preview the render fails with `ErrPhotoUnsupported`
  (`domain.ErrUnsupported`). The image endpoint accepts a photo item ID (`app/images`
  `PhotoSource`), renders photos as JPEG, keys the cache with `artwork.PhotoTag` (path, size,
  mtime) and the width, shares the two-render limit with artwork and keeps current photo
  thumbnails when purging. Wiring (`DetectPhotoDecoders`, `NewPhotoRenderer`, the sqlite
  `PhotoSource`) follows in M3-15e/M3-15h.
- FFmpeg input of folder discs (M3-16f, `internal/adapters/ffmpeg` `DiscInput`,
  `Prober.ProbeDisc`, `Job.Layout`/`Job.DiscTitle` in the producer). This adds to the
  M2-24b whitelist of ADR 0022; single files keep exactly that whitelist.
  - DVD (`VIDEO_TS`): the title VOBs are one program stream cut into files, so they are joined
    byte-wise with the `concat:` protocol: `-f mpeg -format_whitelist mpeg
    -protocol_whitelist file,concat concat:file:…VTS_01_1.VOB|file:…`. The `concat`
    protocol only opens its `file:` parts; no demuxer that `mpeg` allows opens URLs.
  - Blu-ray (`BDMV`): the clips restart their timestamps, so the `concat` demuxer reads a
    list in a new private temporary directory (0700/0600 on Unix; on Windows the ACL of the
    user's temporary directory): `-f concat -safe 0
    -format_whitelist concat -protocol_whitelist file file:…/title.ffconcat`. Each entry
    carries `option format_whitelist mpegts` and `option protocol_whitelist file`, which
    replace the whitelists FFmpeg copies into the clip: a clip whose content is itself an
    ffconcat list is refused (integration test), never followed. Clip durations from the
    MPLS go into `duration` lines, so `-ss` before the input seeks straight into a later clip.
  - `-safe 0` is passed only for lists built from validated parts: an absolute library root
    and a disc folder and clip paths that are clean, local and slash-separated without `|`,
    `\`, line breaks or NUL. On Linux FFmpeg reads every clip through an inherited
    descriptor opened inside the library root (`/proc/self/fd/3…`), like a single file.
  - The list lives as long as the production: it is removed when FFmpeg ends, after `Stop`
    and when the start fails; the prober removes its own after each run.
  - Any ffprobe failure on a title (also a build without the `concat`/`mpeg`/`mpegts`
    demuxers) is `disc_unsupported`. The probed duration is replaced by the title duration
    when the disc structures know one; DVD/Blu-ray subtitles are bitmap streams
    (`media.Stream.IsBitmapSubtitle`).
  - Paths are checked lexically; on Windows and in the prober (no inherited descriptors) a
    clip path is opened by FFmpeg itself, so a symlink swapped in after resolution is
    followed. Library roots are trusted local folders (ADR 0022); Linux productions are not
    affected because the clips are opened through `os.Root`.
- Packaging (M3-16j): Debian's FFmpeg in the Docker image already has everything; the own SPK
  build did not ("Docker and SPK sizes stay unchanged" held only for the image). Recipe revision
  3 (`9.0.2-static3`) adds the `concat` demuxer (`DISC_DEMUXERS`, kept off the library-file
  whitelist) and the `concat` protocol; no new library, 0 to 64 KiB more per binary. One script,
  `packaging/synology/package/scripts/ffmpeg-capabilities.sh`, fails the Docker build, the SPK
  verification (`verify/checks.sh`) and `inspect-spk.sh --capabilities` when the `concat`,
  `mpeg` or `mpegts` demuxer, the `file` or `concat` input protocol or a photo decoder (`mjpeg`,
  `png`, `webp`, `gif`, `tiff`) is missing, and reports `hevc`, HEIF (`mov` plus `hevc`) and
  `libdav1d`: in the image build log and `/usr/local/share/doc/ffmpeg-capabilities.txt`, and on
  the NAS from postinst in `/var/log/packages/<id>.log`. The SPK build is `--disable-network`;
  the `subfile` protocol the ISO input of M3-16g needs was added in recipe revision 4
  (M3-16j-b, below).
- Playlist spans and disc images (M3-16g):
  - `media.DiscClip.Start` carries a Blu-ray play item's IN_time (a timestamp of the clip,
    45 kHz ticks). A clip with a start and a duration gets `inpoint`/`outpoint` (and
    `duration`) lines, so the parts a playlist skips are neither played nor overlap the next
    clip. With copied video the cut lands on a keyframe (FFmpeg); real discs start play items
    at one. In images, clips that lie back to back stay apart for Blu-ray (timestamps restart)
    and merge only for DVD; a clip split over separate extents loses its start.
  - ISO images (`DiscInput.Layout = iso`, `DiscPath` = the image, title = image extents) are
    opened with `os.OpenInRoot`; the iso reader finds `BDMV` (preferred) or `VIDEO_TS` at
    the image root, else `disc_unsupported`; extents beyond the image are `invalid`.
  - FFmpeg reads each extent with the `subfile` protocol:
    `subfile,,start,<first byte>,end,<byte after the last>,,:file:<image>`; it is seekable
    and opens nothing but its inner `file:` URL. A DVD's extents are joined with the
    `concat:` protocol into one program stream (`-f mpeg -format_whitelist mpeg
    -protocol_whitelist file,subfile,concat`); a Blu-ray's extents are the clips of a concat
    list (outer whitelist still `file`), each entry with `option protocol_whitelist
    file,subfile`. On Linux the producer passes the descriptor the image was checked with
    (`file:/proc/self/fd/3`), like a single file; the prober and Windows use the path.
  - Why not the loopback HTTP source the task first planned: it was implemented and passed on
    Windows and Linux, but the bundled SPK FFmpeg (M3-16j) is built with `--disable-network`,
    so `http`/`tcp` do not exist there, and it would have been the only network protocol in
    the whitelists. `subfile` needs no network, no server goroutines and no token.
  - Packaging consequence: an FFmpeg build must include the `subfile` protocol (it is in
    default builds; a minimal build needs `--enable-protocol=subfile`). Without it ffprobe
    fails on the image and the title is `disc_unsupported`.
  - Fallback if `subfile` cannot be shipped: feed the title to FFmpeg on `pipe:0`
    sequentially (`-protocol_whitelist pipe`). It cannot seek, so it would allow only
    transcoding from the start (no resume, no remux seeks); not implemented.
- Packaging (M3-16j-b): M3-16g reads an ISO title as byte ranges of the image through FFmpeg's
  `subfile` protocol (`subfile,,start,A,end,B,,:file:<image>`, protocol whitelist
  `file,subfile`, a DVD title also `concat`) instead of the loopback `http` input, so the SPK
  build stays without network protocols. Recipe revision 4 (`9.0.2-static4`) adds
  `--enable-protocol=subfile` (input protocols exactly concat, file, pipe, subfile; 0 to 64 KiB
  more per binary); `ffmpeg-capabilities.sh` requires `subfile` in every shipped build, Debian's
  FFmpeg in the image (7.1.5) has it. `verify/checks.sh` reads an ISO-like image through
  subfile ranges (DVD title through concat of two ranges, Blu-ray clip through one) and shows
  subfile is refused when it is not on the protocol whitelist. Every ffmpeg call of the
  capability script runs under `timeout(1)` when the system has one, so a hanging binary cannot
  hold up postinst; whether DSM 7.2 ships `timeout` is not documented.
- Scan and playback of discs (M3-16h):
  - A disc entry is one media file: `relative_path` stays what the walker reported (the
    `BDMV`/`VIDEO_TS` folder or the image), `media_files.layout` holds the layout, and
    `naming.PlaceDisc` places it. The title is resolved from the disc root
    (`media.Layout.DiscRoot`: the folder holding `BDMV`/`VIDEO_TS`, so an `AACS` folder next
    to `BDMV` is seen, or the image) and probed; it is not stored. A disc that cannot be
    played is stored with its code as `probe_error` (`disc_encrypted`, `disc_unsupported`,
    `disc_no_title`) and listed as unavailable with that `errorCode`. An unchanged disc
    stored as `disc_unsupported` is probed again by the first scan of every server start,
    since the code also stands for an FFmpeg build that lacks a demuxer or protocol and that
    build only changes with a restart (M3-16k); within one start it counts as unchanged, so
    a corrupt or non-disc image does not fail every scan. Without the disc ports discs are
    skipped (nothing stored, entries at or below them kept) until a scan has the ports; such
    a disc is no unreadable path, so it neither keeps orphan extras nor blocks the metadata
    refresh. Only movies and shows libraries take discs; other kinds drop what the walker
    reports. Discs in the library root stay unmatched.
  - The composition root wires the disc ports for scan and playback (M3-16k): libraryfs
    resolves the disc folder, `disc.Resolver` selects the title and refuses protected discs,
    the ffprobe `Prober` describes the title and finds seek keyframes. The API names a disc
    folder after the folder that holds `VIDEO_TS`/`BDMV` (`MediaFile.fileName`), an image by
    its file name.
  - A session resolves the title again (directory or image resolved inside the library
    root first) and hands it to the producer. The decision engine never plays a disc
    directly: its one source reason is `disc_source` (container and direct-play codec checks
    do not apply), the copy checks still add why a stream is re-encoded, and bitmap
    subtitles keep the `subtitles_not_supported` warning until milestone 4. A disc error at
    start is `domain.ErrUnsupported` (also `disc_no_title`, which would otherwise read as a
    missing file).
  - Remux seeks look for the keyframe in the joined title (`Prober.DiscKeyframeAtOrBefore`,
    the same input as the prober), so `-ss` before the concat input lands on it.
