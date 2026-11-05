# M3-13e Scan: music pipeline
Owner: mivo-backend
Depends on: M3-13a, M3-10b, M3-19i
Files/areas: `internal/app/scan` (new files `music*.go`; one dispatch line in `run.go`), fakes in tests

## Goal
A scan of a `music` folder reads tags, places tracks into artist/album, attaches covers and reports progress.

## Acceptance criteria
- [ ] Ports `AudioTagReader` and `CoverExtractor` declared in `app/scan` (consumer-owned, 1-2 methods); catalog writes go through the existing v2 catalog port plus a small `MusicCatalog` port.
- [ ] Per file: tags -> `Placement`; read failure -> folder fallback and one warning, never a failed scan.
- [ ] Cover order: `cover|folder|front` image in the album folder, then the first track's embedded picture; `artist.jpg` in the artist folder.
- [ ] Unchanged files (size+mtime) are not re-read; removed files are removed.
- [ ] Tests: fakes only; tagged album, compilation, untagged folder, rescan idempotence, cancellation.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Do not reuse the video "no video stream, skip" rule. Keep the pipeline in separate files so M3-14e and M3-15f can add siblings without editing it.

Implementation (M3-13e): ports `AudioTagReader` and `MusicCatalog` in `music_ports.go`, passed
as `Dependencies.Music` (optional; a music library fails with `scan_failed` without them).
`CoverExtractor` is not declared: the prober already reports attached pictures, which become
embedded artwork candidates the image service extracts on demand (ADR 0027 Notes M3-13e), so a
scan-side extractor would be dead code. `pipelineFor` in `run.go` dispatches by library kind;
siblings add one case. Shared edits outside the music files: the `Walker` port passes the
library kind (the walker uses `ClassifyFileIn`), `walked.mediaFiles` holds audio too, the probe
worker pool is a generic `jobBatch` with `describe` split from the no-video rule, `runV2` got
`executeSteps` and a `beforeRefresh` hook, and `artwork.go` routes album and artist items to
`music_artwork.go`. Domain: `naming.PlaceMusicTrack` (disc), `naming.MusicFoldersOf`,
`media.IsVariousArtists`, artwork contexts `album_folder`/`artist_folder`.
