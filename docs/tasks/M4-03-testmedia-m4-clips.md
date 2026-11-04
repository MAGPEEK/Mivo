# M4-03 Corpus: gaps for subtitles, tracks, DSD and HDR
Owner: mivo-media
Depends on: none
Files/areas: `scripts/gen-testmedia/**`, `testdata/` manifest

## Goal
The test corpus contains every input the M4 tasks verify against, built reproducibly (ADR 0021).

## Acceptance criteria
- [ ] After `go run ./scripts/gen-testmedia` the manifest lists, in addition to M1-19: an MKV with
      a styled ASS track plus SRT and WebVTT-convertible `mov_text` in MP4; a file with three audio
      tracks (`eng` stereo default, `deu` 5.1, `eng` commentary titled "Commentary"); a 5.1 AAC clip;
      a 96 kHz/24-bit FLAC and a DSF file (or documented skip when FFmpeg cannot write it).
- [ ] A library folder `testdata/library/subs/` has `Movie (2020).mkv` plus `Movie (2020).en.srt`,
      `.de.forced.srt`, `.fr.sdh.ass` and a `Subs/Movie (2020).es.srt` sidecar set.
- [ ] A 20 s HDR10 clip (exists since M2) is tagged in the manifest for the tone-mapping warning tests.
- [ ] Image subtitles (PGS/VobSub): keep the committed samples of M1-19 and list them under tag `imagesub`.
- [ ] Tests: generator unit tests for the new cases; `go test ./scripts/...`.
- [ ] Gates: lint + coverage thresholds pass.

## Notes
- FFmpeg cannot create PGS; do not try. A DVD-subtitle (`dvdsub`) encoder exists and may produce
  a second image-subtitle sample.
- Keep total corpus size growth under 10 MB.
