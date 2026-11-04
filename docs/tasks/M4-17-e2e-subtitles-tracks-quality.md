# M4-17 E2E: subtitles, audio track and quality
Owner: mivo-frontend
Depends on: M4-15, M4-16b, M4-16c, M4-16d
Files/areas: `web/apps/web/e2e/**`

## Goal
The core streaming-extension flows are proven in a real browser against the real server and the corpus.

## Acceptance criteria
- [ ] Open the `subs` movie, select the German forced SRT: cue text visible in the DOM at a known time, no new session created.
- [ ] Choose the 5.1 German audio track: stream restarts, stream info shows the new track; position preserved within 2 s.
- [ ] Choose "4 Mbit/s": mode becomes transcode, overlay shows the cap, playback continues.
- [ ] A PGS item: selecting the subtitle shows the burn-in hint and `subtitle_burn_in` in the overlay.
- [ ] Hover seek bar after trickplay generated (corpus clip): tooltip image present.
- [ ] Runs in `pnpm -C web e2e` without hardware dependencies.
- [ ] Gates: lint + e2e green.

## Notes
- Chromium shell cannot decode HEVC/AC3; use the H.264/AAC corpus variants for assertions on playback progress.
