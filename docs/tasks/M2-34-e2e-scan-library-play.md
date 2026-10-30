# M2-34 E2E: scan → library → play
Owner: mivo-frontend
Depends on: M2-06, M2-22, M2-23, M2-30, M2-32
Files/areas: `web/apps/web/e2e/**`, `web/apps/web/playwright.config.ts`

## Goal
Playwright drives the real binary through the core flow of milestone 2 – a library appears, a scan
fills it, a video plays and the overlay names the mode – in the bundled Chromium and, where
installed, in a branded browser with proprietary codecs (ADR 0025, OD-7).

## Acceptance criteria
- [ ] Global setup generates a small corpus into the E2E temp root with
      `go run ./scripts/gen-testmedia -only '<list>' -out <root>/media` (at least
      `video-webm-vp9-opus`, `video-mp4-h264-aac-faststart`, `video-mkv-h264-aac`,
      `video-avi-xvid-mp3`) and starts the server with `MIVO_LIBRARIES=<root>/media`. FFmpeg
      missing → fail with a clear message (like Go missing), not a timeout.
- [ ] Spec "library": the sidebar shows the library; starting a scan shows running then finished
      counts; the grid lists the four titles.
- [ ] Spec "play open codec" (bundled Chromium): open the WebM item, Play → the `<video>` reaches
      `currentTime > 1`; the overlay shows "Direct Play" in the direct-play colour.
- [ ] Spec "overlay without proprietary codecs" (bundled Chromium): MKV H.264 shows the translated
      `no_compatible_format` error (Chromium has no H.264) – documents the codec gap explicitly.
- [ ] Project `branded` (`channel: msedge` on Windows, `chrome` elsewhere, override with an env
      variable): MP4 → Direct Play plays; MKV → Remux plays; AVI → Transcode plays (`currentTime >
      1` each) and the overlay shows the mode and a reason; a seek to 50 % on the remux restarts the
      session and playback continues. Skipped with a visible reason when the channel is missing.
- [ ] After the run `<root>/transcode/sessions` is empty (sessions deleted on navigation).
- [ ] axe: no serious/critical findings on the library page and the player.
- [ ] Gates: `pnpm -C web e2e` green on Windows (both projects) and `pnpm -C web lint`.

## Notes
- Clips are 2 s long: assert `currentTime > 1` or `ended`, not long playback.
- Keep the existing M1 specs unchanged; the server is still shared by all specs, so the scan spec
  runs first (serial project dependency or `test.describe.serial`).
- The backlog flake note (BACKLOG "web/e2e") applies: keep `playwright-report` if a failure
  cannot be reproduced.
- (from M2-14) Not verified: newer desktop Chrome may answer `canPlayType('application/vnd.apple.mpegurl')`
  positively, which makes `detectClientProfile` report HLS delivery `native` instead of the
  reference `mse`. Check against the real installed Chrome/Edge and decide (prefer `mse` when
  MediaSource is available) before relying on the reference profile in E2E.
- (from M2-14 test) Playwright's bundled Chromium 156 plays H.264/AAC on the dev machine – check
  whether the branded-channel project (OD-7 C) is still needed; Playwright's Firefox 157 does not
  start here (side-by-side error) – report rather than work around.
