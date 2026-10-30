# M2-30 Web: player page, controls and stream-info overlay
Owner: mivo-frontend
Depends on: M2-27
Files/areas: `web/apps/web/src/features/player/{components,PlayerPage*}/**`,
`web/apps/web/src/features/player/index.ts`, `web/apps/web/src/features/player/locales/{en,de}.json`

## Goal
A full-screen player with its own scrubber over the whole duration and a stream-info overlay that
shows mode, reason, codecs and encoder – the visible proof of "every format plays" (plan screen 4,
ADR 0011 status colours).

## Acceptance criteria
- [ ] `/items/$itemId/play` shows the video full screen with a control bar: back, play/pause,
      scrubber over the probed duration (current position from the timeline, produced range
      shaded for HLS), elapsed/total time, mute + volume, fullscreen, stream-info toggle. Controls
      auto-hide after 3 s of inactivity while playing (named constant).
- [ ] Keyboard: Space/K play-pause, ←/→ ±10 s, F fullscreen, I overlay, M mute, Escape closes the
      overlay or leaves fullscreen; focus stays visible.
- [ ] Stream-info overlay from player-core's `streamInfo`: mode badge in its ui-tokens colour
      (Direct Play green, Direct Stream blue, Transcode amber), translated reasons and warnings
      (e.g. "MKV → remux to fMP4", "Audio AC-3 not supported → AAC"), per-stream lines, container,
      encoder ("Software (libx264)"), source bit rate. Every key exported by player-core has
      en and de text (test).
- [ ] States: loading spinner, `stream_not_ready` waiting text, translated errors for
      `no_compatible_format` and `transcode_unavailable` (the latter says the server can only Direct
      Play and Remux, per the plan), `session_limit_reached` with a retry button.
- [ ] Accessibility: buttons labelled, scrubber is a slider with `aria-valuetext` in h:mm:ss, the
      overlay is a labelled region; axe (component test) has no serious/critical findings.
- [ ] Tests (Vitest + Testing Library): controls call the engine hook correctly (fake hook),
      keyboard map, overlay content for one session per mode, error states. Coverage thresholds
      hold.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
- Native `controls` stay off: the `EVENT` playlist would show only the produced part.
- No subtitles, audio-track switching or quality menu yet (milestone 4); the audio track selector
  can come later through `audioStreamIndex`.
- Colours from ui-tokens only.
