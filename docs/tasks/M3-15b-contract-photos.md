# M3-15b Contract: photos
Owner: mivo-backend
Depends on: M3-14b (sequential edits of `openapi.yaml`)
Files/areas: `api/openapi.yaml`, generated clients, spec test

## Goal
The API describes photo libraries, albums, photos and the EXIF block.

## Acceptance criteria
- [ ] `LibraryKind` gains `photos`; `ItemKind` gains `photo_album|photo`.
- [ ] Item detail gains optional `photo` block (takenAt, make, model, lens, exposureTime, fNumber, iso, focalLengthMm, orientation, latitude, longitude, width, height) and `thumbnailState` (`ready|unsupported`).
- [ ] Generation leaves `git diff --exit-code` clean; spec test asserts the additions.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Photos are artwork of themselves: the image endpoint accepts the photo item id (M3-15d).
