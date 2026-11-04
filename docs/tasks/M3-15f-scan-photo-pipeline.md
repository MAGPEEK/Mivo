# M3-15f Scan: photo pipeline
Owner: mivo-backend
Depends on: M3-15a, M3-13e (dispatch pattern), M3-19i
Files/areas: `internal/app/scan` (new files `photo*.go`; one dispatch line in `run.go`)

## Goal
Scanning a `photos` folder creates albums and photos with EXIF, quickly and without thumbnails.

## Acceptance criteria
- [ ] Ports `PhotoMetadataReader` (path -> `photo.Info`) and `PhotoCatalog` declared in `app/scan`.
- [ ] Per image: EXIF read; failure -> `CaptureTime` falls back to mtime, state `ready` unless the format class is unsupported; one warning per file, scan continues.
- [ ] Videos in photo folders become playable items via the video probe path.
- [ ] Unchanged files (size+mtime) skip EXIF; bounded parallelism (named constant) for large folders.
- [ ] Tests: fakes; nested albums, RAW+JPEG pair, corrupt file, 2 000-file folder stays under the test time budget, cancellation.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Separate files from music/audiobook pipelines. Empty directories produce no album.
