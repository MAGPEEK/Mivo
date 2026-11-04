# M4-12 FFmpeg: trickplay sprite generator
Owner: mivo-media
Depends on: M4-03
Files/areas: `internal/adapters/ffmpeg/trickplay.go`, `trickplay_args.go` (+ tests)

## Goal
One cheap FFmpeg pass produces preview thumbnails for the seek bar (ADR 0030 §8; PL-05).

## Acceptance criteria
- [ ] `GenerateTrickplay(ctx, source, destinationDir, options) (Manifest, error)` runs one process with `-skip_frame nokey` (keyframes only),
      `-vf fps=1/10,scale=160:-2,tile=10x10` writing `sheet-NNN.jpg` (quality 5); `Manifest{TileWidth, TileHeight, Columns, Rows, Interval, SheetCount}`.
- [ ] Runs at low priority (M4-07a), cancellable, bounded at 30 min and 200 sheets; partial output is removed on error; files appear atomically (temp dir + rename).
- [ ] A file with fewer than one tile of duration yields one sheet; audio-only input returns `domain.ErrInvalid`.
- [ ] Tests: arg golden test; corpus test on the 20 s clip (1 sheet, expected rows/columns), cancellation test.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- With keyframes sparser than 10 s tiles repeat; accept, the manifest interval is nominal. The player only needs a nearest tile.
- Disc inputs (M3-16f) pass the same input abstraction; no special casing here.
