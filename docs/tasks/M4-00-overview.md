# Milestone 4 "Streaming-Ausbau" – overview

Scope (plan, milestone 4; order 3 → 6 → 5 → 4 → 7 per OD-UX-2 B): hardware acceleration with a test run, subtitles,
audio-track selection, session management and throttling, bandwidth tiers, DSD/hi-res audio, trickplay, multiple versions in the
UI; deferred: tone mapping, intro detection, chapter images, styled ASS, generated episode stills. Decisions:
[ADR 0030](../adr/0030-streaming-extension-m4.md) (Proposed). Already done and **not** re-planned: the decision engine
(M2-09), HLS arguments and producer (M2-17/24), sessions, idle reaper and limits (M2-20/25), stream info overlay (M2-27/30),
`audioStreamIndex` and `replacesSessionId` (ADR 0024), versions/extras as catalog data (M3-03, ADR 0027).

Baseline fact: the Synology DS925+ has no iGPU. Software transcoding is the product; hardware acceleration is optional and
detected by an actual test encode. Every software default is chosen for 2 cores / 4 threads (ADR 0030 §1).

## Priorities for 0.1.0

| Class | Content | Tasks |
|---|---|---|
| P0 – must be in 0.1.0 | weak-NAS software baseline, throttling, session list, subtitles (text sidecar, burn-in, external files), audio-track choice and defaults, quality steps, HW detection + test | 02a/b, 03, 04a – 04d, 05a/b, 06a/b, 07a – 07c, 08a/b, 09a/b, 11, 13a – 13d, 14a/b, 15, 16a – 16c, 16e, 17, 18 |
| P1 – wanted for 0.1.0, may slip to 0.1.1 | trickplay, DSD/hi-res/5.1 audio, transcoder status page | 10, 12, 13e, 16d, 16f |
| After 0.1.0 | see below | 19 – 23 (reserved ids, not split) |

## Tasks

| ID | Title | Owner | Depends on | Wave |
|---|---|---|---|---|
| [02a](M4-02a-contract-playback-v2.md) | Contract: tracks, subtitles, quality | backend | ADR 0030 | 1 |
| [03](M4-03-testmedia-m4-clips.md) | Corpus gaps | media | – | 1 |
| [04a](M4-04a-domain-track-selection-policy.md) | Domain: default track policy | backend | – | 1 |
| [04c](M4-04c-domain-bitrate-ladder-and-audio-targets.md) | Domain: ladder, audio targets | backend | – | 1 |
| [04d](M4-04d-domain-hardware-encoders.md) | Domain: hardware encoders | backend | – | 1 |
| [05a](M4-05a-config-transcode-keys.md) | Config: transcode keys, safe defaults | backend | – | 1 |
| [05b](M4-05b-domain-external-subtitles.md) | Domain: external subtitles | backend | – | 1 |
| [07b](M4-07b-ffmpeg-progress.md) | FFmpeg: progress | media | – | 1 |
| [07c](M4-07c-ffmpeg-pause-resume.md) | FFmpeg: pause/resume | media | – | 1 |
| [02b](M4-02b-contract-sessions-hwaccel-trickplay.md) | Contract: sessions, transcoder, trickplay | backend | 02a | 2 |
| [04b](M4-04b-domain-subtitle-decision.md) | Domain: subtitle decision | backend | 04a | 2 |
| [06a](M4-06a-scan-external-subtitles.md) | Scan: external subtitles | backend | 05b | 2 |
| [06b](M4-06b-sqlite-external-subtitles.md) | SQLite: external subtitles | backend | 05b | 2 |
| [07a](M4-07a-ffmpeg-software-baseline.md) | FFmpeg: preset, threads, priority | media | 05a | 2 |
| [08a](M4-08a-ffmpeg-subtitle-extraction.md) | FFmpeg: WebVTT extraction | media | 03 | 2 |
| [09a](M4-09a-ffmpeg-hwaccel-detection.md) | FFmpeg: HW detection with test run | media | 04d, 05a | 2 |
| [10](M4-10-ffmpeg-audio-hires-dsd-multichannel.md) | FFmpeg: audio targets | media | 04c, 03 | 2 |
| [12](M4-12-ffmpeg-trickplay-sprites.md) | FFmpeg: trickplay sprites | media | 03 | 2 |
| [16a](M4-16a-player-core-tracks-subtitles-quality.md) | player-core | frontend | 02a, 02b | 2 |
| [08b](M4-08b-ffmpeg-subtitle-burn-in.md) | FFmpeg: burn-in | media | 04b, 07a, 03 | 3 |
| [09b](M4-09b-ffmpeg-hwaccel-arguments.md) | FFmpeg: HW args + fallback | media | 09a, 07a | 3 |
| [11](M4-11-ffmpeg-bitrate-capped-video.md) | FFmpeg: bit-rate cap | media | 04c, 07a | 3 |
| [13a](M4-13a-app-playback-tracks-and-quality.md) | App: playback v2 | backend | 04a – 04d, 02a | 3 |
| [13b](M4-13b-app-sessions-stats-and-throttle.md) | App: list, throttle | backend | 07b, 07c | 3 |
| [13c](M4-13c-app-subtitle-service.md) | App: subtitle service | backend | 08a, 06b, 02a | 3 |
| [13d](M4-13d-app-transcoder-hardware-status.md) | App: transcoder status | backend | 09a, 04d, 05a | 3 |
| [13e](M4-13e-app-trickplay-service.md) | App: trickplay | backend | 12, 02b | 3 |
| [16b](M4-16b-web-playback-options-panel.md) | Web: options panel | frontend | 16a, UX prototype | 3 |
| [16c](M4-16c-web-detail-selectors.md) | Web: detail selectors | frontend | 16a, 02a | 3 |
| [16e](M4-16e-web-stream-info-extensions.md) | Web: overlay extensions | frontend | 02a, 02b | 3 |
| [14a](M4-14a-http-subtitle-and-trickplay-delivery.md) | HTTP: subtitle/trickplay routes | backend | 02a/b, 13c, 13e | 4 |
| [14b](M4-14b-restapi-sessions-transcoder-trickplay.md) | REST: playback v2, sessions, transcoder | backend | 02a/b, 13a, 13b, 13d, 13e | 4 |
| [16d](M4-16d-web-seekbar-trickplay.md) | Web: seek-bar previews | frontend | 16a, 14a | 4 |
| [16f](M4-16f-web-transcoder-status.md) | Web: transcoder status | frontend | 02b, 14b | 4 |
| [15](M4-15-composition-root-streaming.md) | Composition root + integration (**checkpoint**) | backend | all backend/media P0 | 5 |
| [17](M4-17-e2e-subtitles-tracks-quality.md) | E2E | frontend | 15, 16b – 16d | 6 |
| [18](M4-18-nas-verification-and-docs.md) | NAS protocol, docs, release notes | media | 15 | 6 |

Waves run in parallel inside; no two tasks of a wave share a file except as noted below.

## Shared files and sequencing

- `api/openapi.yaml`: 02a → 02b only; nobody else edits it in M4.
- `internal/adapters/ffmpeg/hlsargs.go`: 07a, 08b, 09b, 10, 11 add new files (`burnin.go`, `hwargs.go`, `audioargs.go`, `bitrateargs.go`) and each
  touches the central builder at one call site; run them as listed by wave and rebase. 07a goes first.
- `internal/domain/playback/{plan,decide,transcode}.go`: 04a/b/c/d add new files; 04b, 04c, 04d each edit `decide.go` at one hook; merge sequentially 04a → 04b, 04c, 04d.
- `internal/app/playback`: 13a (start/session), 13b (new files + registry hook) — 13b after 13a in the same wave if conflicts appear.
- Migration number of 06b: take the next free after the second-wave library migrations (`00005`–`00007`).
- `cmd/mivo`, `restapi`: 15 and 14b are last; nobody else edits them.

## Deferred beyond 0.1.0 (reserved ids; split by the architect when scheduled)

| ID | Item | Why it can wait |
|---|---|---|
| M4-19 | Styled ASS in the browser (JASSUB, libass WASM, fonts) | 0.1.0 shows ASS as plain text cues and offers burn-in for styled playback; the WASM runtime adds ~1 MB, font handling and a worker/CSP change that deserves its own review |
| M4-20 | HDR→SDR tone mapping (software `tonemapx`/`zscale`, or `tonemap_vaapi`/`_cuda`) | Software tone mapping of 4K HDR needs several times real time on the DS925+, so it would be unusable exactly where it is wanted; hardware tone mapping needs hardware we cannot test. 0.1.0 keeps ADR 0023's `hdr_not_tone_mapped` warning (and direct play still works on HDR clients). Dolby Vision is already "after v1" |
| M4-21 | Media segments and intro detection, "Skip intro" (PL-07) | Audio fingerprinting is a full-library CPU job (violates the weak-NAS baseline) and adds a dependency (chromaprint); chapter-title based skip may come first. Value is comfort, not correctness |
| M4-22 | Chapter markers on the seek bar and chapter panel/images (PL-30, chapters from M3-14c) | Needs trickplay (M4-12) and chapter extraction for video; low risk to ship later |
| M4-23 | Generated episode stills (DT-23) | Reuses the M3-08 renderer; cosmetic, M6 provides provider stills |

Per-device quality/subtitle settings (AD-02) are implemented as device-local storage inside 16b; server-side per-profile storage is M7.

## Open decisions for the owner (tasks follow the recommendation until answered)

- **ADR 0030 acceptance** (whole document).
- **OD-M4-1 Throttle mechanism**: A `SIGSTOP`/`SIGCONT`, Linux only (recommended) / B stdin `p` key (needs dropping `-nostdin`, behaves differently across FFmpeg builds).
- **OD-M4-2 Default max sessions on ≤ 4 CPUs**: A 1 (recommended) / B keep the current default.
- **OD-M4-3 ASS styling in 0.1.0**: A plain cues, burn-in for styling (recommended) / B pull JASSUB into 0.1.0 (adds M4-19 to P0).
- **OD-M4-4 Trickplay in 0.1.0**: A include as P1 (recommended) / B after 0.1.0.

## Risks

- Burn-in on a DS925+ will not reach real time at 1080p; the plan accepts it (explicit user choice, refused when the limit is reached).
- Hardware paths cannot be verified in CI or on the owner's NAS; only the detection logic and argument builders are tested, real GPUs by contributors (M4-18).
- `SIGSTOP` on a process whose stdout/stderr pipes fill is safe, but the progress reader (07b) must keep draining after resume.
- Testing media: PGS cannot be generated (samples only).

## Agents

| Agent | Tasks |
|---|---|
| mivo-backend | 02a, 02b, 04a – 04d, 05a, 05b, 06a, 06b, 13a – 13e, 14a, 14b, 15 |
| mivo-media | 03, 07a – 07c, 08a, 08b, 09a, 09b, 10, 11, 12, 18 |
| mivo-frontend | 16a – 16f, 17 (16b needs a `mivo-ux` prototype first) |
| mivo-webos, mivo-synology | none (docs input to 18 only) |
