# M5-62 Mode indicator, explanations, overlay extras
Owner: mivo-frontend
Depends on: M5-61
Files/areas: `web/apps/web/src/features/player/components/{ModeBadge,StreamInfoOverlay}.tsx`, `hooks/useStreamInfoOverlay.ts`, `hooks/useNetworkStats.ts`, `lib/reasonHints.ts`, `packages/player-core/src/streamInfo/**`, locales

## Goal
A badge in the player bar shows the delivery mode in its status colour (Direct Play green,
Direct Stream blue, Transcode amber) with the short name; click or `I` opens the overlay;
every reason has a one-line "why" and, where possible, an action hint; the overlay adds buffer
ahead and measured network throughput (PL-40, PL-41 M5 part, PL-42).

## Acceptance criteria
- [ ] Badge updates live when the session changes mode; colour is never the only signal (text +
      icon); full reason on hover and focus.
- [ ] `reasonHints` maps every reason code of the playback API to a translation key; a test
      iterates the OpenAPI enum and fails on a missing mapping or missing en/de string.
- [ ] Hints such as "Choose an AAC track to avoid transcoding" appear only when the action
      exists (hint text without a link until M4's panel).
- [ ] Overlay shows buffer ahead (seconds, from the media element) and throughput (bytes
      per second from HLS fragment/Resource Timing, documented rule) with "unknown" fallback.
- [ ] Tests: lib mapping, throughput maths table tests, badge states, axe.
- [ ] Gates: `pnpm -C web lint`, `pnpm -C web test`, `pnpm -C web build`.

## Notes
Transcode speed, HW encoder name, tone-mapping, burn-in reason arrive with M4; the overlay
renders rows from a list so these just add entries.
