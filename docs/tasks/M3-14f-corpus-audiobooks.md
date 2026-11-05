# M3-14f Corpus: audiobooks
Owner: mivo-media
Depends on: M3-13f
Files/areas: `scripts/gen-testmedia`, manifest, `internal/adapters/corpustest`

## Goal
Generated audiobook fixtures.

## Acceptance criteria
- [x] `library/audiobooks/`: an M4B with 3 chapters and cover, a folder book with 3 MP3 parts (disc/track tags), a single MP3 book in a shared folder.
- [x] Manifest lists title, author, narrator, chapter titles and offsets; corpus test verifies with ffprobe (skips under `-short`).
- [x] Gates: lint + coverage thresholds pass.

## Notes
Reuse the existing chapter case of the corpus where possible.

## Notes (implementation)
- The manifest gained a top-level `audiobooks` object without a schema version bump (like
  `music` in M3-13f and `discs` in M3-16i); existing consumers ignore it.
- Narrator: MP4 has no narrator atom (the ipod muxer drops a custom key), so the M4B books
  carry only a `composer` tag and expect `narrator` empty – a composer is a narrator only when
  `ComposerIsNarrator` is set (M3-14a/M3-14e). The MP3 folder book carries an explicit
  narrator as ID3 TXXX `narrator`. The manifest keeps the raw `composer` apart from the
  `narrator` a scan must produce. A narrator-labelled composer case is left to M3-14g (BACKLOG).
- Chapters are file-relative. `embedded=false` marks the one chapter a scan derives over a
  part without chapters (title tag); its `endSeconds` is the nominal length, the scan ends it
  at the probed length (encoder padding, ±100 ms in the corpus test).
