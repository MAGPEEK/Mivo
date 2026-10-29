# 0005 React + TypeScript + Vite frontend in a pnpm monorepo

- Status: Accepted
- Date: 2026-10-29
- Source: tech stack in `docs/plan.md`

## Context

We need a browser app (embedded in the Go binary) and later an LG webOS app. Both share the
API client, design tokens and playback logic.

## Decision

- **React + TypeScript (strict, no `any`) + Vite.** Node 24 LTS and pnpm 10.
- pnpm monorepo in `web/`:
  - `apps/web`: browser app, embedded into the binary (ADR 0020)
  - `apps/webos`: TV app (milestone 10)
  - `packages/api-client`: generated from `api/openapi.yaml` (ADR 0017)
  - `packages/ui-tokens`: design tokens and `brand.ts` (ADR 0002, ADR 0011)
  - `packages/player-core`: framework-free TypeScript (capabilities, track choice, progress)
- Libraries: TanStack Router, Query and Virtual. Tailwind CSS v4 with Radix UI primitives.
  i18next (ADR 0012). Later hls.js and JASSUB.
- Structure: `src/app` (providers, router, layout), `src/features/<feature>`,
  `src/shared/{ui,lib,i18n}`. Components render. Logic lives in hooks or `player-core`.
- Tests: Vitest + Testing Library. Playwright for end-to-end flows.

## Consequences

- One toolchain for web and TV.
- `player-core` must stay free of React and DOM frameworks so the TV app can reuse it.
  dependency-cruiser enforces this (ADR 0019).
