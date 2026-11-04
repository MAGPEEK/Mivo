# M4-04b Domain: subtitle delivery decision (sidecar or burn-in)
Owner: mivo-backend
Depends on: M4-04a
Files/areas: `internal/domain/playback/subtitles.go`, `plan.go`, `decide.go`, `selection.go` (+ tests)

## Goal
The decision engine chooses per subtitle track between "off", "sidecar" (client renders WebVTT) and
"burn-in", and tells why, replacing the M2 warning `subtitles_not_supported` (ADR 0030 §3).

## Acceptance criteria
- [ ] Profile declares `subtitleFormats` the client renders (default `["webvtt"]` for text); a text
      track the client renders is `sidecar` and does not change the video decision (direct play stays direct play).
- [ ] Image formats and unrendered text formats produce `burn_in`: video is transcoded, reason
      `subtitle_burn_in` with the track format; the mode becomes `transcode`.
- [ ] Burn-in refused (track off, warning `subtitle_burn_in_unavailable`) when the server cannot
      transcode or the profile sets `allowTranscode=false`.
- [ ] `subtitles_not_supported` is no longer emitted for files with subtitles; old tests updated in the matrix.
- [ ] Tests: extend `matrix_test.go` with the six reference profiles × {no sub, SRT, ASS, PGS}; coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Burn-in keeps copy-ineligible video: re-encode needs the source scaled/overlaid, so audio may still be copied.
- Session limit handling stays in `app/playback` (M4-13a).
