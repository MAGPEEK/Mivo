# 0013 webOS 6.0 (2021) is the minimum TV platform

- Status: Accepted
- Date: 2026-10-29
- Source: decision table and webOS section in `docs/plan.md`

## Context

Older webOS releases ship very old Chromium engines and weaker codec support. Supporting them
would hold back the whole TV app.

## Decision

- The TV app supports **webOS 6.0 and newer**.
- The TV build is transpiled for the webOS 6.0 browser engine (Chromium 79 according to LG's
  specs; verify in milestone 10) through `@vitejs/plugin-legacy` and Browserslist.
- Playback uses the native `<video>` element. Each webOS version gets its own client profile
  in the decision engine. DTS is never assumed.

## Consequences

- Shared packages (`api-client`, `ui-tokens`, `player-core`) must not use language or API
  features the TV build cannot transpile or polyfill.
- TVs older than 2021 are unsupported.
