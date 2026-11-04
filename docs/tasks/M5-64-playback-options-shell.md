# M5-64 Playback options shell: auto play, speed
Owner: mivo-frontend
Depends on: M5-61
Files/areas: `web/apps/web/src/features/player/components/PlaybackOptionsPanel.tsx`, `hooks/usePlaybackPreferences.ts`, `lib/playbackSpeed.ts`, locales

## Goal
The options button opens the panel (slide-up on desktop, bottom sheet on phones) with the
M5 sections: Auto play checkbox and Playback speed 0.5-2.0x (PL-14, PL-15). The panel is built
as an ordered list of sections so M4 adds Quality, Audio, Subtitles, Subtitle style (PL-10...13).

## Acceptance criteria
- [ ] Speed steps 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2 applied to the media element; pitch
      preserved; value persisted per device and shown in the panel.
- [ ] Auto play preference persisted per device and read by the next-up logic (M5-66).
- [ ] Escape closes the panel and returns focus to the options button; panel counts as the
      "top panel" for M5-63.
- [ ] Tests: speed lib, preference hook, panel keyboard, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Speed matters most for audiobooks (M5-73): export the speed control as a reusable component.
