# M3-13f Corpus: music tree
Owner: mivo-media
Depends on: M3-17
Files/areas: `scripts/gen-testmedia`, `testdata/` manifest, `internal/adapters/corpustest`

## Goal
Generated, licence-free music for integration tests.

## Acceptance criteria
- [x] `go run ./scripts/gen-testmedia` creates `library/music/`: a tagged FLAC album with embedded cover and disc 1/2, a tagged MP3 album, an M4A album with `cover.jpg` in the folder, a "Various Artists" compilation, an untagged `Artist/Album/NN Title.flac` folder.
- [x] Manifest lists artist, album, disc, track, title and cover source per file.
- [x] Corpus test asserts files exist and FFmpeg reads the tags named in the manifest (skips under `-short`).
- [x] Gates: lint + coverage thresholds pass; generated files stay out of git as before.

## Notes
Tones from FFmpeg `sine`; tags via `-metadata`; cover from a generated PNG. Seconds-long files only.

Implementation: the cover is one generated 500×500 JPEG (not PNG): it is the M4A album's
`cover.jpg` and is copied unchanged into the FLAC tracks as attached picture, so the folder
and the embedded cover are the same bytes (PNG covers are covered by the ffmpeg audio tag
tests). The music catalog is a top-level `music` object in `testdata/library/manifest.json`
(schema stays 1, additive), kept out of `libraries[]` so the movie/show consumers
(`cmd/mivo` library corpus test) are unchanged. The corpus test is
`internal/adapters/corpustest/music_test.go`.
