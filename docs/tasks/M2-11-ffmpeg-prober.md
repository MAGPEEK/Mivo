# M2-11 FFmpeg adapter: process runner, ffprobe prober and keyframe lookup
Owner: mivo-media
Depends on: M2-02
Files/areas: `internal/adapters/ffmpeg/**` (new package: `doc.go`, `process.go`, `probe.go`,
`keyframe.go` + tests, `testdata/probe-*.json`)

## Goal
The server can turn any media file into a `media.Description` by running ffprobe, and can find
the keyframe at or before a position for seeks with stream copy (ADR 0015, ADR 0022, ADR 0024).

## Acceptance criteria
- [ ] A small process runner: `exec.CommandContext`, argument slice, no shell, `-nostdin`-safe
      (stdin closed), stdout read with a size limit (named constant, e.g. 8 MiB → error when
      exceeded), stderr kept as a bounded tail cut at a UTF-8 rune boundary, process killed when
      ctx ends, `WaitDelay` set. Errors include the tool name and exit code, never the full stderr.
- [ ] `Prober.Probe(ctx, absolutePath) (media.Description, error)` runs
      `ffprobe -v error -hide_banner -print_format json -show_format -show_streams file:<path>`
      with a per-file timeout (named constant), parses the JSON and maps it to the domain:
      container via `media.IdentifyContainer`, duration and bit rate (strings, `N/A` tolerated),
      streams with codec, profile, level, pixel format, bit depth, size, frame rate, colour
      fields, HDR, channels, layout, sample rate, `tags.language`/`tags.title`, dispositions
      `default`/`forced`/`attached_pic`.
- [ ] Unit tests on committed JSON fixtures (at least: MKV HEVC10 HDR10 + AC-3, MP4 with
      `mov_text`, multi-track MKV, AVI Xvid, M4B with `bin_data` stream, a file with `N/A`
      values, malformed JSON → error).
- [ ] `KeyframeAtOrBefore(ctx, absolutePath, position) (time.Duration, error)` uses ffprobe with
      `-select_streams v:0 -skip_frame nokey -read_intervals` around the position; falls back to
      a wider interval and finally 0; tested on fixtures and on the long corpus clip.
- [ ] Integration test (skips when ffprobe or `testdata/media/manifest.json` is missing, and
      under `-short`): every generated corpus case is probed and matches the manifest
      expectation (container, probe format, video codec/profile/pixel format/codec tag/HDR/size,
      audio codecs/sample rate/channels, subtitle codecs, track language/default/forced,
      duration ±0.1 s).
- [ ] Hostile file names: a copy of a corpus file named `-dash.mkv` and `Ümlaut ñ 日本.mkv` probes
      correctly (all OSes); on Linux also a name containing `:` (`concat:x.mkv`).
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate
      (package ≥ 80 %).

## Notes
- The ffprobe path comes from config (`tools.ffprobe`) through the constructor; never assume
  `PATH`.
- Keep the JSON structs private to the adapter; only domain types leave it.
- The manifest reader for the integration test is a small private struct (fields you need only);
  `scripts/gen-testmedia` is `package main` and cannot be imported.
- `bits_per_raw_sample` is often missing for video; use `media`'s pixel-format fallback.
- Capability detection (M2-16), argument building (M2-17) and the producer (M2-24) reuse the
  runner from this task; keep it unexported-but-shared inside the package with a clear API.

## Orchestrator note (from M2-02)
- Map ffprobe's `level: -99` (unknown) to 0 – `media.Description.Validate` rejects negative values.
- Derive bit depth with `media.BitDepth(bits_per_raw_sample, pix_fmt)` and the container with
  `media.IdentifyContainer(format_name, extension)`; fill `Stream.CodecTag` from
  `codec_tag_string` (empty for `[0][0][0][0]`).
- Cross-check the container table against the real corpus manifest here (the domain test cannot
  read it).
- A `media.ParseFrameRate` error for one stream (e.g. `r_frame_rate` "25/0") makes that stream's
  frame rate unknown (zero value) – it must not fail the whole probe. Test it.
