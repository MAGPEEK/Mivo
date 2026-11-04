# M3-15g Corpus: photos with EXIF, HEIC and RAW stand-ins
Owner: mivo-media
Depends on: M3-17, M3-15c (byte layout knowledge)
Files/areas: `scripts/gen-testmedia`, manifest, `internal/adapters/corpustest`

## Goal
Generated photo fixtures that close the ADR 0021 gaps.

## Acceptance criteria
- [ ] `library/photos/`: JPEGs with EXIF written by the generator (date, camera, orientation 6, GPS), PNG, WebP, GIF, TIFF, a nested album, a RAW-like TIFF with an embedded JPEG preview, a RAW+JPEG pair, a corrupt JPEG, a short MP4.
- [ ] HEIC/AVIF sample only if FFmpeg in the dev image can encode it (generator skips with a log line otherwise; manifest marks it optional).
- [ ] Manifest lists expected EXIF values and thumbnail state; corpus test checks them (skips under `-short`).
- [ ] Gates: lint + coverage thresholds pass.

## Notes
Own EXIF writer lives in the generator (test-only code), not in the product packages.
