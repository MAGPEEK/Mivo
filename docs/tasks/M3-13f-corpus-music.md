# M3-13f Corpus: music tree
Owner: mivo-media
Depends on: M3-17
Files/areas: `scripts/gen-testmedia`, `testdata/` manifest, `internal/adapters/corpustest`

## Goal
Generated, licence-free music for integration tests.

## Acceptance criteria
- [ ] `go run ./scripts/gen-testmedia` creates `library/music/`: a tagged FLAC album with embedded cover and disc 1/2, a tagged MP3 album, an M4A album with `cover.jpg` in the folder, a "Various Artists" compilation, an untagged `Artist/Album/NN Title.flac` folder.
- [ ] Manifest lists artist, album, disc, track, title and cover source per file.
- [ ] Corpus test asserts files exist and FFmpeg reads the tags named in the manifest (skips under `-short`).
- [ ] Gates: lint + coverage thresholds pass; generated files stay out of git as before.

## Notes
Tones from FFmpeg `sine`; tags via `-metadata`; cover from a generated PNG. Seconds-long files only.
