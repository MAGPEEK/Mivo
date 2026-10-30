# M2-21 player-core: playback timeline, seek planning and stream-info model
Owner: mivo-frontend
Depends on: M2-14
Files/areas: `web/packages/player-core/src/{timeline,seek,streamInfo}/**`,
`web/packages/player-core/src/index.ts`

## Goal
Framework-free logic that the web player (and later the TV app) uses to show the right position
for restarted HLS sessions, to decide between a local seek and a session restart, and to turn a
session into the stream-info overlay content (ADR 0024).

## Acceptance criteria
- [ ] Timeline: from a session (actual start, duration, delivery) and the element's `currentTime`
      compute the displayed position, clamped to `[0, duration]`; and the inverse for local seeks.
- [ ] `planSeek(target, state)` returns `{kind: "local", time}` when the target lies in the
      playable range (direct play: always; HLS: inside the seekable/buffered ranges passed in) and
      `{kind: "restart", position}` otherwise; positions are clamped.
- [ ] `shouldRecreateSession(error)` is true for HTTP 404/410 on a session URL and for hls.js
      fatal network errors on segments; false otherwise.
- [ ] `streamInfo(session)` returns a view model without strings: mode, colour token name
      (`directPlay`, `directStream`, `transcode` from ui-tokens), translation keys plus params for
      each reason and warning (unknown codes → generic key), per-stream lines (source → target,
      copy/transcode, formatted with the M2-14 label functions), container change, encoder,
      source bit rate.
- [ ] Types match the API schema `PlaybackSession` structurally (no api-client import).
- [ ] Table tests; coverage of player-core stays ≥ 90 %.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
- ui-tokens may be imported (framework-free) for the token names; return the name, not the hex.
- Translation keys are the contract with the player feature's locale files (M2-30); list them
  in an exported constant so the app can test that every key is translated.
