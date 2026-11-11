# M4-16a player-core: track selection, subtitle cues, quality, trickplay maths
Owner: mivo-frontend
Depends on: M4-02a, M4-02b
Files/areas: `web/packages/player-core/src/**` (framework-free, + tests)

## Goal
All player logic for ADR 0030 lives in `player-core` and is unit-tested without a browser (CLAUDE.md: no business logic in components).

## Acceptance criteria
- [ ] Client profile gets `preferredAudioLanguages`, `preferredSubtitleLanguages`, `subtitleMode`, `subtitleFormats: ["webvtt"]`, `maxAudioChannels` (from
      `navigator`/MediaCapabilities probing), bounded; session requests carry `audioStreamIndex`, `subtitleTrackId`, `maxBitRate`.
- [ ] `parseWebVtt(text)` returns cues (start, end, plain text, line/position ignored); `activeCues(cues, fileTime)`; the engine maps `fileTime = actualStart + currentTime`;
      a track switch for text subtitles never restarts the stream.
- [ ] Switching audio or a burn-in subtitle or quality restarts through `replacesSessionId` at the current position (existing restart helper).
- [ ] `qualityOptions(sourceBitRate, height)` returns Original + ladder steps below the source; `AutoQuality` picks a step from measured segment throughput
      (EMA, 0.8 safety factor, 30 s hold-down before stepping up).
- [ ] `trickplayTile(manifest, seconds)` returns sheet, column, row; out-of-range clamps.
- [ ] Subtitle format labels move here (BACKLOG row from M2-23).
- [ ] Tests: Vitest table tests for every function; coverage thresholds.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`.

## Notes
- No DOM access in this package; the web app renders the cues.
