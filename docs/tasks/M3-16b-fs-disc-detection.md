# M3-16b FS: disc folder and ISO detection in the walker
Owner: mivo-media
Depends on: M3-16a, M3-07b
Files/areas: `internal/adapters/fs/libraryfs` (new file `disc.go`, one hook in the walk)

## Goal
The walker reports each disc as one entry instead of its inner files.

## Acceptance criteria
- [ ] A directory containing `VIDEO_TS/VIDEO_TS.IFO` or `BDMV/index.bdmv` (case-insensitive, also when the parent itself is `VIDEO_TS`/`BDMV`) yields one disc entry with the layout; its contents are not walked as separate videos.
- [ ] `.iso` files yield an `iso` entry; size limit sanity check (> 0, regular file); symlinks outside the root are ignored (existing root guard).
- [ ] Extras folders and other files beside the disc folder still behave as before.
- [ ] Tests: temp-dir integration tests, case variants, nested disc in a movie folder, ISO beside NFO/poster; Windows and Linux paths.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The scan consumes the entry in M3-16h; walker output only grows by a layout field.
