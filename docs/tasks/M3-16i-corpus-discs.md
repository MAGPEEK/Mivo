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

Implementation (2026-11-05): discs live in their own root `testdata/library/discs` (manifest
top-level `discs`, schema version unchanged); FFmpeg writes the streams to
`testdata/library/.disc-sources`, the generator assembles VIDEO_TS, BDMV, an ISO 9660 image
of the DVD and the CSS/AACS stand-ins (flags only, no keys, nothing encrypted). Real bitmap
subtitles: the generator writes a raw PGS stream (FFmpeg cannot render text to bitmaps); the
Blu-ray copies it (`hdmv_pgs_subtitle`), the DVD re-encodes it with `dvdsub`. The corpus test
plays the DVD title through the decision engine and the FFmpeg producer; playing it through
the session API and the `scripts/playback-smoke` row need the disc scan and wiring of
M3-16h/M3-16k and are left to those tasks. The ISO title is checked by selection and extent
bytes only; probing it needs the loopback input of M3-16g.
