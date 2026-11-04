# M3-08 FFmpeg: artwork renderer (resize, embedded pictures)
Owner: mivo-media
Depends on: M3-04b
Files/areas: `internal/adapters/ffmpeg/image.go`, `image_args.go` (new) + tests
(`image_internal_test.go`, `image_corpus_test.go`), the shared input-whitelist helper only if
it needs a new argument (no change to the existing whitelist values)

## Goal
The server turns a local poster, backdrop, logo or an embedded cover into a small JPEG or PNG
of an allowed width with the FFmpeg it already bundles, safely and with bounded time and
memory, so the image endpoint can serve posters on a 1 GB NAS. UX: LB-44, LB-40; ADR 0027 §8,
OD-M3-2 A.

## Acceptance criteria
- [ ] `ImageRenderer.Render(ctx, artwork.RenderRequest) (artwork.Rendered, error)`: scales to
      `MaxWidth` keeping the aspect ratio (height even), **never upscales**, outputs JPEG with
      a named quality constant or PNG with alpha for `artwork.FormatPNG`, returns width and
      height of the output. `StreamIndex >= 0` extracts that attached-picture stream of a
      video file (`-map 0:<index>`), `-1` reads the whole image file.
- [ ] Every invocation passes `-protocol_whitelist file` and the input `-format_whitelist` of
      ADR 0022 (image2 only for paths without `%`; a `%` path is refused for image sources
      with `domain.ErrUnsupported`), the Windows device-path allowlist (M2-39), a decoder
      `-max_pixels` limit (64 MP, named constant), a single output frame (`-frames:v 1`), and
      writes to stdout (`pipe:1`) – no temporary files.
- [ ] Limits: output larger than 8 MiB → error; render timeout 15 s (named constant, via
      ctx); FFmpeg missing → `domain.ErrUnavailable`; undecodable or over-limit input →
      `domain.ErrUnsupported`; cancellation → `context.Canceled`. Error texts carry base name,
      tool and exit code only.
- [ ] The source file size limit (32 MiB) is checked before FFmpeg starts.
- [ ] Unit tests of the argument builder (table: JPEG, PNG, embedded stream, `%` path,
      device path); fake-tool tests for timeout, missing tool, oversized output; corpus tests
      with real FFmpeg on the image cases of `testdata/media` and the artwork of
      `testdata/library` (M3-17; skip when absent): JPEG/PNG/WebP source, PNG with alpha →
      PNG output keeps alpha, a 320-px source requested at 1920 stays 320, an MKV cover
      attachment and an MP4 `covr` extract.
- [ ] Docker check: the corpus tests pass with the image's FFmpeg 7.1 (Docker line of
      `CLAUDE.md`), like M2-16/M2-17.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/ffmpeg/...`, coverage gate.

## Notes
- Scaling filter: `scale='min(iw,W)':-2` with a high-quality flag (`lanczos` or `bicubic`);
  JPEG via `mjpeg` with `-q:v`, PNG via `png`; strip metadata (`-map_metadata -1`).
- Use the existing process runner and lifetime handling (kill on cancel, Windows job objects)
  of the package; do not start FFmpeg any other way.
- The renderer does not cache and does not limit concurrency; M3-09 does both.
