# M3-15a Domain: photo kind, image file classes, photo info
Owner: mivo-backend
Depends on: M3-04, M3-19b
Files/areas: `internal/domain/library` (new files), `internal/domain/photo` (new package), `internal/domain/naming` (new file `photo.go`)

## Goal
Pure rules for photo albums, photos, capture date and orientation.

## Acceptance criteria
- [ ] `library.KindPhotos`; item kinds `photo_album|photo`; image extension classes: web (`jpg jpeg png webp gif avif`), `heic heif`, `tiff tif`, RAW (`cr2 cr3 nef arw dng orf rw2 raf`); video files in a photo library are classified as video.
- [ ] `photo.Info` (taken at, make, model, lens, exposure, ISO, focal length, orientation 1-8, GPS lat/lon optional, width, height) with `CaptureTime(info, modTime)` falling back to the file modification time.
- [ ] Placement: each directory is a `photo_album` following the folder tree; each image a `photo`; sort by capture time then file name.
- [ ] RAW+JPEG pair with the same stem in one folder: the JPEG is the item, the RAW is its alternate file.
- [ ] Tests: table-driven, domain coverage >= 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No I/O. Decision OD-M3-4 = B (ADR 0029).
