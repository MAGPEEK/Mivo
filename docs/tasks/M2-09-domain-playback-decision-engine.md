# M2-09 Domain: playback decision engine and reference profiles
Owner: mivo-media
Depends on: M2-02, M2-01 (`domain.ErrUnsupported`)
Files/areas: `internal/domain/playback/**` (new package),
`internal/domain/playback/playbacktest/**` (new package)

## Goal
Given a probed file, a client profile and the server's capabilities, a pure function picks
Direct Play, Remux or Transcode, the action per stream and the reasons the overlay shows
(ADR 0023). Reference profiles make the decision reproducible in every test layer (ADR 0025).

## Acceptance criteria
- [ ] Types: `ClientProfile` (name, direct-play entries, HLS support with delivery
      `mse|native|none`, codec limits) with `Validate()` (bounded lengths, lower-case identifiers;
      `domain.ErrInvalid`), `ServerCapabilities` (HLS muxing, H.264 and AAC encoders, FFmpeg
      version), `Request` (selected audio index, start position), `Policy` (max transcode height),
      `Plan` (mode, delivery, target container, per-stream decisions, reasons, warnings),
      `StreamDecision` (source index, action copy/transcode, source and target codec, target
      size/channels).
- [ ] `Decide(input) (Plan, error)` implements the ADR 0023 rules in order: stream selection,
      direct play, remux, transcode; reason and warning codes exactly as listed in ADR 0023.
- [ ] Errors wrap `domain.ErrUnsupported` and expose their code (`no_compatible_format`,
      `transcode_unavailable`) through exported sentinels `ErrNoCompatibleFormat` and
      `ErrTranscodeUnavailable`; `AudioByIndex` misses wrap `domain.ErrInvalid`.
- [ ] Transcode targets: H.264 High 8-bit 4:2:0 and AAC LC stereo; downscale only when the
      source height exceeds `Policy.MaxTranscodeHeight` (even width, aspect kept); warning
      `video_downscaled`; HDR source transcoded → `hdr_not_tone_mapped`.
- [ ] `playbacktest` provides `ChromiumDesktop()` and `FirefoxDesktop()` profiles with exactly
      the values of the ADR 0025 table and
      `FullCapabilities()` / `NoEncoders()` server capabilities, each with a test (coverage gate).
- [ ] Table test "format matrix × profiles": one row per video case of the corpus (container +
      codecs from `cases_video.go`, the subtitle and multi-track MKVs, the long clips) × both
      reference profiles × both capability sets, with the expected mode and reasons. Includes:
      attached-picture skipped, non-default audio selected → `audio_track_not_default`, 10-bit
      H.264 → transcode, `hls.delivery = none` → only direct play or `no_compatible_format`.
- [ ] Coverage ≥ 90 % for both packages; no mocks.
- [ ] Gates: lint (funlen, gocognit ≤ 15) + layer rules + coverage gate pass.

## Notes
- Keep rules as small named functions (direct-play check, copy check, transcode targets) to stay
  under the complexity limit; the reason list is built by the checks, not by a big switch.
- FFmpeg presets, CRF and arguments are not domain logic (M2-17).
- The matrix rows double as documentation of expected modes; M2-33 keeps its own table keyed by
  manifest case names and must agree with these rows.
- The transcode-height default comes from OD-5 through `Policy`; the engine has no default of
  its own beyond "0 = no cap".
