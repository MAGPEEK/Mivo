# M3-16i Corpus: generated discs, integration test and playback smoke
Owner: mivo-media
Depends on: M3-17, M3-16c
Files/areas: `scripts/gen-testmedia`, manifest, `internal/adapters/corpustest`, `scripts/playback-smoke` (one profile row)

## Goal
Licence-free disc fixtures covering clear and encrypted cases.

## Acceptance criteria
- [ ] Generated `Movie (2019)/VIDEO_TS` (FFmpeg `dvd` muxer output split into `VTS_01_1.VOB..` plus a minimal hand-written IFO), `BDMV` (mpegts m2ts clips and a generated MPLS), an `.iso` built by the generator, a "scrambled" VOB/AACS stand-in with the flags of M3-16d.
- [ ] Manifest records layout, expected main-title duration and expected error code.
- [ ] Corpus test asserts detection, title selection and probe results (skips under `-short`); playback smoke plays one VIDEO_TS title through the session API.
- [ ] Gates: lint + coverage thresholds pass.

## Notes
Seconds-long streams only. The generator's IFO/MPLS writers are test-only code.
