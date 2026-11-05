# M3-16g FFmpeg: ISO input through the subfile protocol
Owner: mivo-media
Depends on: M3-16e, M3-16f
Files/areas: `internal/adapters/ffmpeg` (new file `iso_input.go`), `internal/adapters/disc/iso` (read-only use)

## Goal
Let FFmpeg read clips inside an ISO without mounting and without any network protocol.

## Acceptance criteria
- [x] The image is opened inside the library root (`os.OpenInRoot`); the iso reader detects
  BDMV (preferred) or VIDEO_TS at the image root, else `disc_unsupported`; title extents beyond
  the image are `invalid`.
- [x] FFmpeg reads each title extent as `subfile,,start,<first>,end,<after last>,,:file:<image>`:
  a DVD's extents joined by the `concat:` protocol into one program stream, a Blu-ray's extents
  as clips of a concat list. On Linux the producer hands FFmpeg the descriptor the image was
  checked with (`file:/proc/self/fd/3`).
- [x] Whitelists add only `subfile` (and the `concat` protocol for DVD) to `file`; no network
  protocol can reach FFmpeg. A build without `subfile` fails the probe as `disc_unsupported`.
- [x] Seeking (`-ss` before the input, remux into a later clip) works; nothing outlives the run
  except the concat list, which is removed as for folder discs.
- [x] Tests: unit tests for the image input (arguments, list, failures, symlink out of the root,
  cancellation); integration tests with generated ISO 9660 images (probe, remux after a seek,
  transcode; skip under `-short`); the corpus ISO title is probed (`corpustest`).
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Deviation (recorded 2026-11-05)
The task first asked for a per-session loopback HTTP range server (127.0.0.1, random port and
path token, `http` whitelisted only for that form). It was built and passed on Windows and
Linux, then replaced before review: the bundled SPK FFmpeg (M3-16j) is built with
`--disable-network`, so `http`/`tcp` do not exist on the NAS, and `subfile` reads the same
byte ranges seekably without a server, goroutines, token or network protocol. The SPK build
still has to enable `subfile` (follow-up M3-16j-b, see BACKLOG). Details: ADR 0029 Notes M3-16g.

## Notes
Fallback if `subfile` cannot be shipped: FFmpeg `pipe:0` fed sequentially (no seeking;
transcode-from-start only), documented in ADR 0029 Notes.
