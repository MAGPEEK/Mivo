# M4-08b FFmpeg: burn subtitles into the transcode
Owner: mivo-media
Depends on: M4-04b, M4-07a, M4-03
Files/areas: `internal/adapters/ffmpeg/hlsargs.go` (new `burnin.go`), `producer.go` (+ tests), `packaging/docker` fontconfig note (BACKLOG row "docker HOME/XDG_CACHE_HOME")

## Goal
Image subtitles (PGS, VobSub, DVB) and unrendered text tracks appear in the picture when the plan says `burn_in` (ADR 0030 §3).

## Acceptance criteria
- [ ] Image tracks: `-filter_complex "[0:v][0:s:<n>]overlay"` (overlay at source size, then scale to the output size, so the bitmap keeps its place on the downscaled video; order amended by the orchestrator 2026-11-12; the old wording said scale first, which would misplace the overlay: it matches
      downscaled video); text tracks: `subtitles=` / `ass=` filter with an escaped path (no raw user path in the filter string) and
      a fonts directory option.
- [ ] With `-ss` seeking the subtitle stream is seeked as well (`-ss` before `-i` covers both); a test seeks 30 s into the PGS sample.
- [ ] Hardware encoders (M4-09b) receive the burn-in graph on software frames (`hwupload` after overlay); documented in the arg builder. (Amended 2026-11-12: `burnInGraph` takes the `uploadFilter` tail; passing it on the hardware path is M4-09b's integration task, with a table test that the tail follows the overlay.)
- [ ] FFmpeg without libass (`-filters` lacks `subtitles`) makes `ServerCapabilities` report `BurnInText=false`; image overlay needs only `overlay`.
- [ ] Tests: arg table tests; corpus tests: PGS sample, styled ASS sample and an external SRT burn in (frame difference via a single decoded frame
      compare against the same frame without subtitle).
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Escape rules for filter paths (`:`, `\`, `'`) are the classic failure; fuzz-style table of hostile names, and copy the file to a temp
  name in the session directory if escaping is not provable.
- Fonts: no system fonts guaranteed in the Docker image; use the font set of the image (`fontconfig` + one open font) — coordinate with packaging.
