# 0025 Playback verification: reference profiles, expected modes, smoke and E2E

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-30
- Refines: ADR 0021 (test media corpus), "Verifikation" in `docs/plan.md`

## Context

Milestone 2 promises that every video container of the corpus plays in the browser. The
decision depends on the client profile and the server's FFmpeg; the output depends on the
FFmpeg version (9.0.2 on the dev machine, Debian's 7.1.x in the Docker image). Playwright's
bundled Chromium has no proprietary codecs (H.264, AAC, HEVC).

## Decision

- **Reference profiles** live in `internal/domain/playback/playbacktest` as Go values:
  `ChromiumDesktop` (H.264/AAC/VP9/AV1/Opus, MSE HLS, no Matroska, no Theora, no HEVC) and
  `FirefoxDesktop`. They mirror what player-core reports for those browsers; player-core tests use
  fake probe answers that produce the same profiles. The package has its own test so it meets
  the domain coverage gate. Their exact values (the single source for Go and TypeScript tests):

  | | ChromiumDesktop (`chromium-desktop`) | FirefoxDesktop (`firefox-desktop`) |
  |---|---|---|
  | directPlay `mp4` | video `h264 av1 vp9`, audio `aac mp3 opus flac` | same |
  | directPlay `webm` | video `vp8 vp9 av1`, audio `opus vorbis` | same |
  | directPlay `ogg` | – | video `theora`, audio `vorbis opus flac` |
  | hls | `mse`, video `h264 av1 vp9`, audio `aac mp3 opus flac` | same |
  | codecLimits | `h264`: maxBitDepth 8, profiles `Constrained Baseline, Baseline, Main, High`; `vp9`, `av1`: maxBitDepth 10 | same |

  Both deliberately omit HEVC, AC-3/E-AC-3 and Matroska, which real browsers support only on some
  platforms.
- **Expected modes** per corpus case and reference profile live in Go, in
  `internal/adapters/corpustest` (a non-test file, so `scripts/` can import it), keyed by the
  manifest case name. They are not added to `manifest.json`: the manifest describes files, the
  mode depends on profile and server. A test fails when a generated case with a video stream
  has no expectation, so the table grows with the corpus.
- **Integration test** (`go test ./internal/adapters/corpustest/...`): scans the generated corpus
  with the real walker, prober and SQLite (temp dir), creates a session per video case with each
  reference profile, waits for FFmpeg to finish, joins `init.mp4` and the segments and checks the
  result with ffprobe (codecs per the plan, duration within half a second of the manifest, start
  near the requested position for seek cases). It also runs with server capabilities without
  encoders and expects `transcode_unavailable` exactly for the transcode cases. It skips cleanly
  when FFmpeg or the corpus is missing, and under `-short`.
- **Seek cases**: the corpus gains two 30-second clips with a 2-second keyframe interval (one to
  remux, one to transcode).
- **Smoke test against a running server**: `go run ./scripts/playback-smoke` drives the HTTP API
  of a dev server or the Docker image (scan, sessions, downloads, local ffprobe) with the same
  expectations. It is the release gate for the image's FFmpeg version.
- **E2E** (OD-7 C, decided 2026-10-30): Playwright's bundled Chromium checks scan → library → play with an open
  codec (VP9/Opus direct play) and the stream-info overlay for a remux and a transcode case; a
  second project on a branded browser channel (`msedge` on Windows, `chrome` elsewhere) checks
  that H.264 remux and transcode actually play. It is skipped with a clear message when the
  channel is not installed.

## Consequences

- One table answers "which mode should this file get" for the integration test, the smoke test
  and reviewers.
- `go test ./...` takes longer with FFmpeg installed (all video cases are remuxed or
  transcoded); `-short` skips it.
- Results on the Docker image's FFmpeg are only checked by the smoke test, not by `go test`.
