# M2-41 Player: pause intent, scrubber listeners, focus, mode-neutral reason text
Owner: mivo-frontend
Depends on: M2-30, M2-34 (committed)
Files/areas: `web/apps/web/src/features/player/**` only: `hooks/useScrubber.ts`,
`hooks/usePlayer.ts`, `hooks/usePlaybackSession.ts`, `hooks/useControlsVisibility.ts`,
`lib/playbackEngine.ts`, `locales/en.json`, `locales/de.json` and their tests
(`*.test.ts(x)`, `test/**`); `web/apps/web/e2e/player.spec.ts` (line ~132) and
`web/apps/web/e2e/branded.spec.ts` (line ~52) only for the assertions of the old reason text;
`docs/tasks/BACKLOG.md` (only the rows below). Not
`web/packages/player-core` (no change needed there).

## Goal
A user who paused the video stays paused when the player recovers from a network error or
retries a stream that was not ready, and the Retry button respects a pause chosen in the error
state. Dragging the scrubber twice leaks no window listeners, keyboard focus reliably keeps the
controls visible, and the stream-info overlay no longer claims "remux" for a transcode.

Closes BACKLOG rows (delete them from `docs/tasks/BACKLOG.md` when done, touch no other row):
- `player` | "M2-30 review r2 leftovers: `useScrubber` `listen()` leaks …; ADR 0024 note that
  a keyboard Retry pins the controls" (M2-30 review r2)
- `player locales` | "`container_not_supported` reads '… → remux to {{targetContainer}}' …"
  (M2-34 review)

Already done by the architect: the ADR 0024 note "Player intent and Retry" in "Notes (M2-37 to
M2-41)" (including the keyboard-Retry note). Report any deviation instead of editing the ADR.

## Acceptance criteria
- [ ] `useScrubber`: a second `pointerdown` before the drag ended removes the previous
      `pointerup`/`pointercancel` pair before adding the new one. Test: two `pointerdown`s, one
      `pointerup` → exactly one seek, and spies on `window.addEventListener` /
      `removeEventListener` show no listener left after the drag and after unmount.
- [ ] Play/pause while a stream plays also sets the engine's intent (`autoplay`). Engine tests:
      pause while playing, then (a) a recovery restart after a fatal hls.js network error and
      (b) a `stream_not_ready` retry → the new stream is attached but not played and the
      snapshot shows `autoplay: false`; play again → it plays. The play button still acts on
      the element at once while playing (existing tests green).
- [ ] `retry()` restarts with the current intent instead of always playing: after an error,
      setting the intent to paused and pressing Retry attaches the new stream without playing;
      with the default intent Retry plays (existing retry tests green).
- [ ] `useKeyboardFocusInControls` re-checks `:focus-visible` of `document.activeElement` on
      `keydown` as well as on `focusin`, so a control focused by a mouse click that then gets
      keyboard input pins the controls. Test with a stubbed `matches(":focus-visible")` (jsdom)
      for both directions (becomes visible on keydown; not pinned before).
- [ ] Locales: `streamInfo.reason.container_not_supported` is mode-neutral in both languages:
      en `{{sourceContainer}} not supported → {{targetContainer}}`, de
      `{{sourceContainer}} nicht unterstützt → {{targetContainer}}`. Test in
      `streamInfoLocales.test.ts`: for a transcode session with `container_not_supported` the
      rendered reason contains neither "remux" nor "Remux" in en or de; the same check for a
      remux session; no reason or warning text of either locale contains a mode word
      (`remux`, `transcode`, case-insensitive).
- [ ] Tests: `pnpm -C web test` green with the coverage thresholds; `pnpm -C web e2e` green
      in both projects (bundled Chromium and the branded channel, ADR 0025).
- [ ] Gates: `pnpm -C web lint` (ESLint, dependency-cruiser, Prettier, tsc) passes.

## Notes
- Simplest intent fix: while `status === "playing"`, `playControl` calls `setAutoplay(!paused)`
  instead of acting on the element only; `PlaybackEngine.setAutoplay` already plays or pauses
  an attached element and publishes the intent. Check that the engine's own pause/play events
  (e.g. the browser pausing at the end) do not overwrite the intent unexpectedly; document the
  choice in a why-comment.
- `retry()` uses `this.autoplay` as `restart` already does for recoveries (playbackEngine.ts
  `restart(request, target, this.autoplay)`).
- The old text is asserted in `PlayerPage.overlay.test.tsx` (lines ~71, ~81),
  `e2e/player.spec.ts:132` and `e2e/branded.spec.ts:52`; update only those assertions
  (`MKV not supported → MP4`, `FLV not supported → MP4`).
- No business logic in components: the intent rule belongs in the engine/hooks.
