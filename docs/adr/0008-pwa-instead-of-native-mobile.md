# 0008 PWA instead of native mobile apps

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

Native iOS and Android apps would double client effort and need store accounts and reviews.

## Decision

- Mobile access works through the web app as a **Progressive Web App** (manifest, icons,
  responsive layout with bottom navigation on phones, installable).
- No native mobile apps for now. A future native app needs a new ADR.

## Consequences

- One codebase for desktop and mobile browsers.
- Background playback and offline sync are limited by browser capabilities. That is accepted
  for v1.
