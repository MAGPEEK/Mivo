# M2-27 Web: player engine – sessions, hls.js and seeking by restart
Owner: mivo-frontend
Depends on: M2-01, M2-07, M2-21
Files/areas: `web/apps/web/src/features/player/{hooks,lib,api}/**` (+ tests),
`web/apps/web/package.json`, `web/pnpm-lock.yaml` (adds `hls.js`; the only lockfile change in
wave 4)

## Goal
Opening the player creates a playback session with the browser's profile and plays it – a file
for Direct Play, HLS through hls.js (or natively in Safari) for remux and transcode – with seeks
that restart the session when needed and clean teardown (ADR 0023, ADR 0024).

## Acceptance criteria
- [ ] `hls.js` added with an **exact** version (no `^`/`~`), loaded with a dynamic `import()` so
      the main bundle does not grow; the build report shows it in its own chunk.
- [ ] Profile: computed once per page load with `detectClientProfile` from real browser probes;
      a compile-time check proves player-core's `ClientProfile` is assignable to the API schema
      type (and the session type to player-core's).
- [ ] `usePlaybackSession({itemId, fileId, startAt})`: `POST /api/v1/playback/sessions`, then
      attaches to a `<video>` element: `file` → `src`; `hls` + `mse` → hls.js `loadSource` /
      `attachMedia`; `hls` + `native` → `src`. Exposes state (`loading|playing|error`), the session
      and the displayed position via the player-core timeline.
- [ ] Seek: `planSeek` → local `currentTime` or a new session with `startPositionSeconds` and
      `replacesSessionId`; the old hls.js instance is destroyed before the new one attaches.
- [ ] Recovery: when `shouldRecreateSession` says so, a new session at the current position is
      created once; a second failure surfaces as an error.
- [ ] Teardown on unmount/navigation: hls.js destroyed, `DELETE` sent with `fetch(…, {keepalive:
      true})`; also on `pagehide`.
- [ ] Errors are typed for the UI: `no_compatible_format`, `transcode_unavailable`,
      `session_limit_reached` (with retry), `stream_not_ready` (auto-retry with the `Retry-After`
      value), generic.
- [ ] Tests (Vitest) with a fake API and an injected fake hls factory (no real media in jsdom):
      each delivery path, seek local vs restart, recovery once, teardown calls, error mapping.
      Coverage thresholds hold.
- [ ] Gates: `pnpm -C web install --frozen-lockfile` after the change, `pnpm -C web lint`,
      `pnpm -C web test`, `pnpm -C web build`; `docker build -f packaging/docker/Dockerfile .`
      still succeeds (lockfile in the image build).

## Notes
- Components stay thin: all logic in hooks or player-core. The page UI is M2-30.
- hls.js config: keep defaults except `enableWorker` (keep true) and a bounded
  `maxBufferLength`; note in the REPORT that a future CSP (BACKLOG) needs `worker-src blob:`.
- Never build stream URLs on the client; use `streamUrl` from the session (ADR 0024).

## Orchestrator note (from M2-07)
- Feature translation namespaces are registered in `web/apps/web/src/app/i18n.ts` (one line per
  feature), not in `shared/i18n/resources.ts`; the i18next type augmentation lives in
  `src/app/i18next.d.ts`. Features only edit their own `locales/{en,de}.json`.
- Pages receive route params as props (`LibraryPage {libraryId}`, `ItemPage {itemId}`,
  `PlayerPage {itemId, fileId, startAt}`); `features/player/lib/playerSearch.ts`
  (`validatePlayerSearch`) already exists.
- (from M2-07 test) `validatePlayerSearch` accepts any non-negative `t` (also 1e21) and any
  non-empty `fileId` string (no UUID check, no length cap). Treat both as untrusted: clamp `t` to
  the item duration and let the API reject unknown file ids; never build paths from them.
