# M3-14e Scan: audiobook pipeline
Owner: mivo-backend
Depends on: M3-14a, M3-13e
Files/areas: `internal/app/scan` (new files `audiobook*.go`; one dispatch line in `run.go`)

## Goal
Scanning an `audiobooks` folder creates books with author, narrator, cover and chapters.

## Acceptance criteria
- [x] Port `ChapterReader` declared in `app/scan`; reuses `AudioTagReader` from M3-13e (M3-13e declares no `CoverExtractor`: embedded covers are attached-picture streams the prober reports, see ADR 0027 Notes M3-13e).
- [x] Single-file book: chapters from the file; folder book: file chapters if present, else one chapter per part.
- [x] Cover rules as for albums; author/narrator stored as people.
- [x] Tests: fakes; M4B with chapters, folder with 3 parts, mixed folder, rescan, cancellation.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Separate files from the music pipeline to avoid edit conflicts.
