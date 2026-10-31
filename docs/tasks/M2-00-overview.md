# Milestone 2 "Durchstich Video" – overview

Scope (plan, milestone 2): scan a folder → ffprobe → library in the web UI → play via Direct Play,
Remux (fMP4/HLS) or software Transcode, with the stream-info overlay showing mode and reason.
Afterwards every video container of the test corpus plays in the browser. Together with the Docker
image this is release v0.1-alpha.

Not in scope: login (M7), name parser / watcher / NFO / artwork (M3), hardware acceleration,
subtitles, tone mapping, throttling, bandwidth tiers (M4), online metadata (M6).

## New ADRs (accepted by the project owner on 2026-10-30)

| ADR | Decision |
|---|---|
| [0022](../adr/0022-media-library-model-and-scanning.md) | Config-defined libraries, data model, one item per file, confined scanning, change detection, path safety, new config keys |
| [0023](../adr/0023-playback-decision-engine-and-client-profiles.md) | Pure decision engine, identifiers, client profile schema, mode rules, transcode targets, reason codes, capability detection |
| [0024](../adr/0024-streaming-sessions-and-delivery.md) | Sessions, direct file and fMP4 HLS delivery, seeking by restart, idle timeout, limits, transcode dir purge, readiness for signed URLs (M7/M7b) |
| [0025](../adr/0025-playback-verification.md) | Reference profiles, expected modes in Go, corpus integration test, smoke test against Docker, E2E browser projects |

## Tasks

| ID | Title | Owner | Depends on | Wave |
|---|---|---|---|---|
| [M2-01](M2-01-api-contract-video.md) | API contract for libraries, items, playback | mivo-backend | – | 1 |
| [M2-02](M2-02-domain-media.md) | Domain: technical media description | mivo-media | – | 1 |
| [M2-03](M2-03-domain-library.md) | Domain: libraries, items, file policies | mivo-backend | – | 1 |
| [M2-04](M2-04-config-libraries-tools-transcode.md) | Config: libraries, tools, transcode limit | mivo-backend | – | 1 |
| [M2-05](M2-05-transcode-workspace.md) | Transcode workspace (session dirs, purge) | mivo-backend | – | 1 |
| [M2-06](M2-06-testmedia-seek-clips.md) | Corpus: seek clips, `-only` lists, manifest fixes | mivo-media | – | 1 |
| [M2-07](M2-07-web-routes-and-i18n-namespaces.md) | Web: routes + per-feature translations | mivo-frontend | – | 1 |
| [M2-08](M2-08-http-logtext-and-stream-readiness.md) | HTTP: logtext, ReaderFrom, quiet stream logs, nits | mivo-backend | 01 | 2 |
| [M2-09](M2-09-domain-playback-decision-engine.md) | Domain: decision engine + reference profiles | mivo-media | 01, 02 | 2 |
| [M2-10](M2-10-sqlite-schema-and-libraries.md) | SQLite: schema + library repository | mivo-backend | 02, 03 | 2 |
| [M2-11](M2-11-ffmpeg-prober.md) | FFmpeg: runner, prober, keyframe lookup | mivo-media | 02 | 2 |
| [M2-12](M2-12-library-walker.md) | FS: confined walker + resolver | mivo-backend | 01, 03 | 2 |
| [M2-13](M2-13-single-instance-lock.md) | Single instance per data dir | mivo-backend | – (go.mod) | 2 |
| [M2-14](M2-14-player-core-client-profile.md) | player-core: client profile + labels | mivo-frontend | 01 | 2 |
| [M2-15](M2-15-sqlite-media-catalog.md) | SQLite: media catalog repository | mivo-backend | 10 | 3 |
| [M2-16](M2-16-ffmpeg-capabilities.md) | FFmpeg: capability detection | mivo-media | 09, 11 | 3 |
| [M2-17](M2-17-ffmpeg-hls-arguments.md) | FFmpeg: HLS arguments | mivo-media | 09, 11 | 3 |
| [M2-18](M2-18-app-library-catalog.md) | App: library catalog | mivo-backend | 02, 03 | 3 |
| [M2-19](M2-19-app-scan.md) | App: background scan | mivo-backend | 02, 03 | 3 |
| [M2-20](M2-20-app-playback-sessions.md) | App: playback sessions | mivo-media | 09 | 3 |
| [M2-21](M2-21-player-core-timeline.md) | player-core: timeline, seek, stream info | mivo-frontend | 14 | 3 |
| [M2-22](M2-22-web-library-ui.md) | Web: sidebar libraries, grid, scan | mivo-frontend | 01, 07, 14 | 3 |
| [M2-23](M2-23-web-item-detail.md) | Web: item detail with chips | mivo-frontend | 01, 07, 14 | 3 |
| [M2-24](M2-24-ffmpeg-stream-producer.md) | FFmpeg: HLS stream producer | mivo-media | 11, 17, 20 | 4 |
| [M2-25](M2-25-app-playback-lifecycle.md) | App: session lifecycle | mivo-media | 20 | 4 |
| [M2-26](M2-26-restapi-library-endpoints.md) | REST: libraries, scans, items | mivo-backend | 01, 08, 18, 19 | 4 |
| [M2-27](M2-27-web-player-engine.md) | Web: player engine + hls.js (pnpm-lock) | mivo-frontend | 01, 07, 21 | 4 |
| [M2-28](M2-28-restapi-playback-endpoints.md) | REST: playback sessions | mivo-backend | 01, 08, 20 | 4 |
| [M2-29](M2-29-stream-delivery-handlers.md) | HTTP: direct file + HLS delivery | mivo-backend | 01, 05, 08, 12, 20 | 4 |
| [M2-31](M2-31-lan-only-guard.md) | LAN-only guard (only if OD-10 accepted) | mivo-backend | 04, 08 | 4 |
| [M2-30](M2-30-web-player-ui.md) | Web: player UI + stream-info overlay | mivo-frontend | 27 | 5 |
| [M2-32](M2-32-composition-root-video.md) | Composition root: wire it all | mivo-backend | see file | 5 |
| [M2-33](M2-33-corpus-playback-integration.md) | Integration: every corpus video plays | mivo-media | see file | 5 |
| [M2-34](M2-34-e2e-scan-library-play.md) | E2E: scan → library → play | mivo-frontend | 06, 22, 23, 30, 32 | 6 |
| [M2-35](M2-35-playback-smoke-and-docker.md) | Smoke script + Docker verification | mivo-media | 32, 33 | 6 |
| [M2-37](M2-37-hls-playlist-from-memory.md) | HLS playlist from memory, start at 0 | mivo-media | 29, 32 | 6b |
| [M2-38](M2-38-playback-limits-and-lifecycle-nits.md) | Finished sessions free FFmpeg slots; lifecycle nits | mivo-backend | 25 | 6b |
| [M2-39](M2-39-ffmpeg-prober-logger-and-device-paths.md) | FFmpeg: prober logger, device-path allowlist | mivo-media | 11, 16, 24b, 32 | 6b |
| [M2-40](M2-40-scan-catalog-contract-and-link-walk.md) | Scan: catalog contract, `scan_canceled`, link walk | mivo-backend | 12, 15, 19 | 6b |
| [M2-41](M2-41-player-review-leftovers.md) | Player: pause intent, scrubber, focus, reason copy | mivo-frontend | 30, 34 | 6b |
| [M2-36](M2-36-docs-and-release-readiness.md) | Docs + release readiness | mivo-backend | all, incl. 37–41 | 7 |

## Waves (parallel within a wave; no two tasks of a wave share a file)

1. M2-01 ‖ M2-02 ‖ M2-03 ‖ M2-04 ‖ M2-05 ‖ M2-06 ‖ M2-07 – go.mod: M2-01 only
2. M2-08 ‖ M2-09 ‖ M2-10 ‖ M2-11 ‖ M2-12 ‖ M2-13 ‖ M2-14 – go.mod: M2-13 only
3. M2-15 ‖ M2-16 ‖ M2-17 ‖ M2-18 ‖ M2-19 ‖ M2-20 ‖ M2-21 ‖ M2-22 ‖ M2-23
4. M2-24 ‖ M2-25 ‖ M2-26 ‖ M2-27 ‖ M2-28 ‖ M2-29 ‖ M2-31 – pnpm-lock: M2-27 only
5. M2-30 ‖ M2-32 ‖ M2-33 – **walking-skeleton checkpoint after M2-32**: `go run -tags webui
   ./cmd/mivo` with a library scans, lists and plays one file per mode
6. M2-34 ‖ M2-35
6b. M2-37 ‖ M2-38 ‖ M2-39 ‖ M2-40 ‖ M2-41 – the BACKLOG rows due before 0.1.0-alpha.2
   (planned 2026-10-31). Disjoint files; the only shared file is `docs/tasks/BACKLOG.md`, where
   each task deletes only its own rows. M2-39 edits one line of
   `internal/adapters/http/stream/corpus_test.go`, M2-37 owns the rest of that package.
7. M2-36, then the release decision (OD-9) and `mivo-release` with explicit user approval

Same-package pairs that are safe because they touch disjoint files: M2-16/M2-17
(`adapters/ffmpeg`), M2-25 with M2-28/M2-29 (they only consume `app/playback`), M2-26/M2-28
(`restapi/library*` vs `restapi/playback*` + `problem.go`). Shared web test helpers (`src/test/**`)
belong to M2-22 in wave 3; M2-23 keeps its fakes inside its feature.

Why the skeleton is wired in wave 5 and not earlier: the contract (M2-01) is fixed in wave 1, so
every layer is tested end to end against it with fakes from wave 2 on, and the first wiring
needs the producer and the stream handler anyway. Wiring a direct-play-only skeleton earlier
would mean editing `cmd/mivo` twice for about one wave of gain.

## Decisions of the project owner (2026-10-30)

All open decisions below were answered with the recommended option: OD-1 A (config-defined
libraries, Docker presets `MIVO_LIBRARIES=/media`), OD-2 A, OD-3 A, OD-4 A (2 re-encoding
sessions, cap 8, reject with retry), OD-5 B (max 1080p, no upscaling), OD-6 A, OD-7 C, OD-8 A,
OD-9 A (`0.1.0-alpha.2`, amd64 + arm64, no armv7), OD-10 A (M2-31 is in scope), OD-11 A.
ADRs 0022–0025 are accepted. Approvals granted: network downloads (Go modules, npm `hls.js`,
Docker base images, govulncheck database), the `CLAUDE.md` Commands edit in M2-36, and the
installed Microsoft Edge / Google Chrome as Playwright channels. Every Docker push still needs
the owner's explicit approval per push.

## Approvals the orchestrator needs from the user

- Network: module download for `github.com/oapi-codegen/runtime` (M2-01), `golang.org/x/sys`
  promotion (M2-13), `hls.js` from npm (M2-27), Docker base images (M2-16 fixture capture, M2-17
  FFmpeg 7.1 check, M2-32, M2-35), govulncheck database (all Go tasks).
- M2-36 edits the "Commands" section of `CLAUDE.md`.
- M2-34 uses the installed Microsoft Edge through Playwright's `msedge` channel (no download).
- Acceptance of ADRs 0022–0025 and the open decisions below. Tasks follow the recommendations
  until answered; tasks affected by a different answer are named per decision.

## Open decisions for the user

**OD-1 – How are libraries defined in v0.1-alpha?** (affects M2-01, M2-04, M2-18, M2-22, M2-32,
M2-35)
- A: In `config.yaml` / `MIVO_LIBRARIES`; the API and UI only show them. **Recommended**: without
  login, an "add library" form would let any device in the network publish any server folder.
  The UI form comes with the setup wizard in M7.
- B: Add/remove libraries through API and UI now, without authentication.
- C: Only one fixed library at `/media` (Docker) – too narrow for native Windows installs.
- Sub-question: should the Docker image preset `MIVO_LIBRARIES=/media`, so mounting `/media` is
  enough? **Recommended: yes.**

**OD-2 – Library kinds and item model in M2** (affects M2-03, M2-04, M2-22)
- A: Accept `movies` and `shows` in the config now, but list every video flat, one item per file,
  title from the file name; hierarchy and name parser in M3. **Recommended** (config written for
  the alpha stays valid in M3).
- B: Single generic `videos` kind until M3, then migrate.
- C: Pull the name parser (title, year, SxxEyy) into M2 – roughly three more tasks.

**OD-3 – When does a scan run in M2?** (affects M2-19, M2-32)
- A: Button in the UI plus once at every server start. **Recommended.**
- B: Button only.
- C: Additionally every N hours (the watcher and periodic scans are planned for M3).

**OD-4 – Transcode concurrency** (affects M2-04, M2-20, ADR 0024)
- A: `transcode.max_sessions` counts only sessions that re-encode (default **2**), plus a fixed
  safety cap of 8 FFmpeg sessions including remux. **Recommended**: remux is cheap and should not
  block other viewers; two software transcodes already saturate a typical NAS CPU.
- B: One limit for all FFmpeg sessions (default 2) – simplest, but blocks a third remux viewer.
- C: Default derived from CPU cores (e.g. half the cores, 1–4).
- When the limit is reached: reject the new session with a clear message and retry button
  (recommended) or stop the oldest session.

**OD-5 – Output resolution when transcoding** (affects M2-09 policy, M2-32)
- A: Keep the source resolution.
- B: Downscale to at most 1080p, never upscale. **Recommended**: a 4K software transcode is not
  real time on NAS CPUs; bandwidth tiers come in M4.
- C: At most 720p for weak NAS.

**OD-6 – Which browsers are release targets for v0.1-alpha?** (affects M2-14 tests, M2-34,
README)
- A: Chrome/Edge and Firefox on desktop, current and previous major version, are supported; Safari
  (macOS/iOS) "should work, community-tested" because there is no Apple test device in the plan.
  **Recommended.**
- B: Safari fully supported – needs a Mac or iPhone for testing.
- C: Chromium-based browsers only.

**OD-7 – Browser for E2E playback** (affects M2-34, ADR 0025)
- A: Playwright's bundled Chromium only – cannot play H.264/AAC, so remux/transcode playback stays
  untested in E2E.
- B: Branded Edge/Chrome only – every runner needs it installed.
- C: Both: bundled Chromium for the flow and open codecs, branded channel for H.264 remux and
  transcode, skipped with a message when not installed. **Recommended.**

**OD-8 – Symlinks pointing outside a library folder** (affects M2-12, ADR 0022)
- A: Skip and log. **Recommended** (no way to escape the configured folders; NAS users add the
  target as its own library).
- B: Follow them.

**OD-9 – Release after M2** (affects M2-36 and the `mivo-release` run)
- A: Version `0.1.0-alpha.2`, image tags `0.1.0-alpha.2` + `alpha` (no `latest`), linux/amd64 +
  linux/arm64, git tag `v0.1.0-alpha.2`. **Recommended** (continues the M1 alpha line).
- B: Call it `0.1.0` ("v0.1-alpha" by milestone, not by tag).
- armv7: publish only after one smoke run on armv7 (QEMU or a device). **Recommended: not in this
  release.**

**OD-10 – LAN-only guard until accounts exist (M7)** (affects M2-31, M2-32, README)
- A: Refuse requests from public IP addresses by default (`server.lan_only`, opt-out), health
  endpoint excluded. **Recommended**: protects users who forward the port by mistake; private
  ranges, Docker bridges, Tailscale and LAN reverse proxies keep working.
- B: No guard, only a warning in the README and docs.

**OD-11 – Audio when transcoding** (affects M2-09, M2-17)
- A: AAC stereo. **Recommended** for M2: plays everywhere; surround passthrough and multichannel
  AAC belong to M4's streaming work.
- B: AAC with up to 5.1 channels when the source has them.

## Parallel track: Synology package (SPK)

Since 2026-10-31 the native Synology package runs in parallel (owner decision, ADR 0026
Accepted, overview [SPK-00](SPK-00-overview.md)). It touches only `packaging/synology/**` and
changes no milestone-2 file. It reuses M2-32 (wired server) and M2-35 (smoke script) in SPK-11
and attaches its SPKs to the `v0.1.0-alpha.2` GitHub Release (SPK-13); it must not delay the
release (OD-SPK-1).

## Risks and notes

- Playwright's bundled Chromium has no proprietary codecs (OD-7); without a branded channel the
  E2E suite cannot prove H.264 playback.
- FFmpeg differs between the dev machine (9.0.2) and the image (Debian 7.1.x): M2-16, M2-17 and
  M2-35 check the image explicitly.
- HDR sources are transcoded without tone mapping and look washed out until M4 (warning
  `hdr_not_tone_mapped` in the overlay).
- No throttling: a long transcode fills `/transcode` up to the output size; documented in M2-35.
- `go test ./...` runs FFmpeg for the whole corpus (M2-33); `-short` skips it.
- Windows keeps files of a killed FFmpeg locked for a moment; M2-05 retries deletion.
- 36 tasks: plan for several review rounds; stop rule (3 failed rounds) per task as usual.
