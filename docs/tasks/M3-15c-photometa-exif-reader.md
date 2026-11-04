# M3-15c Photometa adapter: pure-Go EXIF reader and RAW/HEIC preview extraction
Owner: mivo-media
Depends on: M3-15a
Files/areas: `internal/adapters/photometa` (new package), fuzz and unit tests, tiny fixtures under the package's `testdata`

## Goal
Read EXIF without bundling exiftool, and fetch the embedded JPEG preview of RAW files.

## Acceptance criteria
- [ ] Reads EXIF/TIFF IFDs (IFD0, Exif, GPS) from JPEG (APP1), TIFF-based RAW (CR2, NEF, ARW, DNG, ORF, RW2), and ISOBMFF `Exif` items (HEIC/HEIF/AVIF); CR3/RAF handled when the layout is simple, else `ErrUnsupported`.
- [ ] Returns `photo.Info`; missing tags are zero values, never errors; corrupt data returns an error without panic.
- [ ] `ExtractPreview(ctx, path)` returns the largest embedded JPEG (SubIFD/IFD0 `JPEGInterchangeFormat` or the RAF/CR3 preview box), bounded by a size constant.
- [ ] Bounded reads (max header bytes), no path escape: the caller passes an opened file or a validated path.
- [ ] Tests: table tests with synthetic byte fixtures, one fuzz test per parser, no network.
- [ ] Gates: lint + layer rules + coverage thresholds pass; no new go.mod requirement.

## Notes
Own minimal parser (stdlib `encoding/binary`) to avoid unmaintained EXIF libraries; GPS as rationals -> decimal degrees. Port is consumer-owned (`PhotoMetadataReader`, `PreviewExtractor`) in M3-15f/15d.
