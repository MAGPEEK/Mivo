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

## Orchestrator note (from M2-07)
- Feature translation namespaces are registered in `web/apps/web/src/app/i18n.ts` (one line per
  feature), not in `shared/i18n/resources.ts`; the i18next type augmentation lives in
  `src/app/i18next.d.ts`. Features only edit their own `locales/{en,de}.json`.
- Pages receive route params as props (`LibraryPage {libraryId}`, `ItemPage {itemId}`,
  `PlayerPage {itemId, fileId, startAt}`); `features/player/lib/playerSearch.ts`
  (`validatePlayerSearch`) already exists.
- M2-30 may change `PlayerRoute` in `web/apps/web/src/app/router.ts` to pass the whole validated
  `PlayerSearch` as one prop (and map `t` → `startAt` inside the feature), so later player search
  params (e.g. audio stream index in M4) no longer need router edits. Also fix the router comment:
  the root is the only route with a `notFoundComponent`, so every unmatched path ends there.
- (from M2-07 test) `validatePlayerSearch` accepts any non-negative `t` (also 1e21) and any
  non-empty `fileId` string (no UUID check, no length cap). Treat both as untrusted: clamp `t` to
  the item duration and let the API reject unknown file ids; never build paths from them.
- (from M2-21) Stream-info texts are translation keys relative to the player namespace under
  `streamInfo.*` (27 keys, exported as `streamInfoTranslationKeys`); add a `streamInfo` block to
  `features/player/locales/{en,de}.json` and a test that every exported key exists in both. Unknown
  codes map to `<group>.unknown`; params always include `code`. `StreamInfoColorToken` is a local
  union in player-core – add a compile-time check in the app that it is assignable to
  `@mivo/ui-tokens` `ColorToken`.

## Orchestrator note (from the M2-27 review r2)
- The engine keeps the old attachment only until a restart is requested; decide here whether to keep
  it playing until the new session is created (needs suppressing the old stream's position updates
  and audio).
- Engine leftovers to fix while building the page: `continueAfter` must not clamp a waiting target
  of another file with the late session's duration (playbackEngine.ts:323 – clamp only for the same
  request/fileId); native HLS errors before the first frame on a `ready` session must end as a
  generic error once the retries run out (playbackEngine.ts:514-546); tests for suspend/resume and
  open-another-file while a create is in flight; a seek on an attached but not yet loaded direct-play
  file should seek locally; publish the session in `continueAfter` so the stream-info overlay never
  shows a stale one.
