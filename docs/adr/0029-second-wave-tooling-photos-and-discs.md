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
- ISO: a pure-Go UDF/ISO 9660 reader serves clips through a per-session loopback HTTP range
  source; the `http` protocol is allowed only for that loopback form. ISO may ship one release
  after folder support.
- Whitelist additions are exactly: `concat` format, `file` protocol for the list file, and the
  loopback `http` form. All other protocols stay blocked (extends the M2-24b whitelist).
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
