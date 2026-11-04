# M3-15d Photo thumbnails: FFmpeg rendering, capability probe, image endpoint source
Owner: mivo-media
Depends on: M3-15a, M3-15c, M3-08, M3-09b
Files/areas: `internal/adapters/ffmpeg` (new file `photo_render.go`), `internal/app/images` (source `photo`, new file), tests

## Goal
On-demand thumbnails for photos through the existing image endpoint and cache.

## Acceptance criteria
- [ ] Renderer scales JPEG/PNG/WebP/GIF/TIFF/AVIF with FFmpeg, applying EXIF orientation (autorotate) and the existing `-max_pixels` guard; widths from the ADR 0027 §8 set.
- [ ] RAW: preview from `ExtractPreview` is piped to FFmpeg; HEIC/AVIF: used only if the startup capability probe (`ffmpeg -decoders`/`-demuxers`) shows support, otherwise the preview image of the container, otherwise state `unsupported`.
- [ ] Unsupported or failing sources return a typed error the endpoint maps to the existing "no artwork" response; never fails a scan.
- [ ] Source switch in `app/images` resolves a photo item to its main file; the cache key includes mtime and width.
- [ ] Tests: integration with generated JPEG (rotated EXIF), PNG, WebP; fakes for RAW preview and the capability probe; skip FFmpeg tests under `-short`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Thumbnails are never rendered during the scan. Concurrency limit shared with artwork rendering.
