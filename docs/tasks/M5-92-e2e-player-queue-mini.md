# M5-92 E2E: player, queue, mini player
Owner: mivo-frontend
Depends on: M5-65, M5-67
Files/areas: `web/apps/web/e2e/player-layout.spec.ts`, `queue.spec.ts`, `mini-player.spec.ts`

## Goal
Playwright proves the new player layout, queue and mini player on real playback of the corpus.

## Acceptance criteria
- [ ] Play an episode: mode badge shows the expected mode, `I` opens the overlay with reasons,
      keys (Space, arrows, F, M, ?) work, help sheet lists them.
- [ ] Season Play queues episodes; Next advances; reorder in the queue panel; auto play
      next-up countdown can be cancelled.
- [ ] Minimise keeps playback running across navigation (currentTime advances), expand returns
      to the player, close stops the session (server sessions list empty via `/activity`).
- [ ] Speed change alters `playbackRate`.
- [ ] Gates: `pnpm -C web e2e` green.

## Notes
Use the same corpus profiles as `scripts/playback-smoke` (chromium, firefox where installed).
