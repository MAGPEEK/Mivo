# M3-14f Corpus: audiobooks
Owner: mivo-media
Depends on: M3-13f
Files/areas: `scripts/gen-testmedia`, manifest, `internal/adapters/corpustest`

## Goal
Generated audiobook fixtures.

## Acceptance criteria
- [ ] `library/audiobooks/`: an M4B with 3 chapters and cover, a folder book with 3 MP3 parts (disc/track tags), a single MP3 book in a shared folder.
- [ ] Manifest lists title, author, narrator, chapter titles and offsets; corpus test verifies with ffprobe (skips under `-short`).
- [ ] Gates: lint + coverage thresholds pass.

## Notes
Reuse the existing chapter case of the corpus where possible.
