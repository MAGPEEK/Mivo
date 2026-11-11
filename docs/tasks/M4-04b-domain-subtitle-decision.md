# M4-04b Domain: subtitle delivery decision (sidecar or burn-in)
Owner: mivo-backend
Depends on: M4-04a
Files/areas: `internal/domain/playback/subtitles.go`, `plan.go`, `decide.go`, `selection.go` (+ tests)

## Goal
The decision engine chooses per subtitle track between "off", "sidecar" (client renders WebVTT) and
"burn-in", and tells why, replacing the M2 warning `subtitles_not_supported` (ADR 0030 §3).

## Acceptance criteria
- [x] Profile declares `subtitleFormats` the client renders (omitted or empty = the client renders
      none, contract 1.16.0; the web client sends `["webvtt"]` from M4-16a); a text track the client
      renders is `sidecar` and does not change the video decision (direct play stays direct play).
- [x] Image formats and unrendered text formats produce `burn_in` only for an explicitly requested
      track: video is transcoded, reason `subtitle_burn_in`; the mode becomes `transcode`. The track
      format is carried in `Plan.Subtitle.Format` (a `Reason` has only code and scope).
- [x] Burn-in impossible (server cannot transcode, or no H.264/HLS target for the client): with
      `allowDegradeSubtitles` the track is off with warning `subtitle_burn_in_unavailable`, without it
      the decision fails (client limits before server limits). There is no `allowTranscode` field in
      the contract.
- [x] `subtitles_not_supported` is no longer emitted for files with subtitles; old tests updated in the matrix.
- [x] Tests: extend `matrix_test.go` with the reference profiles that exist (Chromium and Firefox
      desktop) × {no sub, SRT, ASS, PGS} × MP4/MKV × capabilities × HLS delivery; further reference
      profiles (webOS) extend the matrix when they are added; coverage ≥ 90 %.

Criteria amended by the orchestrator on 2026-11-11 to match contract 1.16.0 and ADR 0030 Notes
(M4-04b); the original wording predated the M4-02a/b contract decisions.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Burn-in keeps copy-ineligible video: re-encode needs the source scaled/overlaid, so audio may still be copied.
- Session limit handling stays in `app/playback` (M4-13a).
