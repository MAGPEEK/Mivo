# M2-14 player-core: detect the browser's client profile; technical labels
Owner: mivo-frontend
Depends on: M2-01 (schema `ClientProfile`)
Files/areas: `web/packages/player-core/src/**` (replace `capabilities.ts` /
`containerSupport.ts` scaffolding, new `profile/**`), `web/packages/player-core/src/index.ts`

## Goal
The web app can tell the server exactly what this browser plays – progressive containers,
codecs inside HLS, bit depths – so the decision engine picks the cheapest working mode
(ADR 0023).

## Acceptance criteria
- [ ] `detectClientProfile(probes, options) → Promise<ClientProfile>` with injected probes:
      `canPlayType` (media element), `isTypeSupported` (MediaSource, optional), `decodingInfo`
      (MediaCapabilities, optional), plus a `name`. No DOM globals are read inside player-core.
- [ ] Probe tables (ADR 0023): container keys `mp4 mov webm mkv ogg mpegts 3gp` with their MIME
      types; video codecs `h264 hevc av1 vp9 vp8 theora` and audio codecs `aac mp3 opus vorbis
      flac ac3 eac3` with representative RFC 6381 strings, incl. 10-bit variants for HEVC, AV1,
      VP9 and H.264 High 10 to derive `codecLimits.maxBitDepth`.
- [ ] `canPlayType` `probably`/`maybe` count as supported; with `decodingInfo` available a codec
      must also report `supported: true`. HLS delivery: `native` when
      `application/vnd.apple.mpegurl` plays, else `mse` when `isTypeSupported` exists, else `none`.
- [ ] The `ClientProfile` type is structurally identical to the API schema (no import of
      `@mivo/api-client`; M2-27 adds a compile-time assignability check in the app).
- [ ] Tests with fake probes reproducing Chromium, Firefox, Safari and "no MSE" answers; the
      Chromium and Firefox results equal the reference profiles in the table of ADR 0025 (kept
      as JSON fixtures in player-core with a comment pointing to the ADR; M2-09 implements the
      same values in Go).
- [ ] Technical labels (`src/labels/**`), shared by the library grid, the detail page and the
      overlay so they never format codecs differently: `containerLabel` (`mkv` → `MKV`),
      `codecLabel` (`h264` → `H.264`, `hevc` → `HEVC`, `mpeg2video` → `MPEG-2`, `eac3` →
      `E-AC-3`, `pcm_s16le` → `PCM`, unknown → upper-case id), `channelLabel` (1 → `1.0`,
      2 → `2.0`, 6 → `5.1`, 8 → `7.1`), `hdrLabel`, `bitDepthLabel`, `resolutionLabel`
      (height → `480p`/`720p`/`1080p`/`4K`). Language-neutral, table-tested.
- [ ] Coverage of `player-core` ≥ 90 % (ADR 0019).
- [ ] Gates: `pnpm -C web lint` (incl. dependency-cruiser: no React, no api-client import),
      `pnpm -C web test`, `pnpm -C web build`.

## Notes
- Chromium's `canPlayType("video/x-matroska")` is empty although Chromium plays many MKVs; follow
  the probe (ADR 0023 consequence), do not special-case browsers.
- Keep the probe tables as data so the TV app (milestone 10) can reuse them.
- `decodingInfo` is async and may throw for unknown types: treat a rejection as unsupported.
