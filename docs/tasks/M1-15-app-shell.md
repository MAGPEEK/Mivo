# M1-15 App shell: sidebar, top bar, empty home, server version
Owner: mivo-frontend
Depends on: M1-13, M1-14
Files/areas: `web/apps/web/src/app/**` (layout, providers), `web/apps/web/src/shared/ui/**`,
`web/apps/web/src/features/home/**`, `web/apps/web/src/features/system/**`,
`web/apps/web/src/shared/i18n/locales/*.json`

## Goal
Opening the app shows the Mivo shell (ADR 0011): a collapsible left sidebar, a top bar with
a search field (Ctrl+K), an empty home page, and the server version loaded live from
`/api/v1/system/info`. This is the frontend end of the walking skeleton.

## Acceptance criteria
- [ ] Layout uses landmarks: `<nav>` (sidebar, labelled), `<header>` (top bar), `<main>`
      (page content).
- [ ] Sidebar: product name from brand, "Home" link (active state with `aria-current="page"`),
      a "Libraries" section showing "No libraries yet", and a collapse toggle button with
      `aria-expanded` and `aria-controls`. The collapsed state persists in `localStorage`
      under a key derived from `brand.productId`.
- [ ] Top bar: labelled search input (visible placeholder, visually hidden label).
      `Ctrl+K` / `Cmd+K` focuses it and prevents the browser default. Search does nothing
      yet.
- [ ] Home: heading plus an empty state ("Your library is empty" plus a hint) using token
      colours.
- [ ] Sidebar footer shows the server version from `useSystemInfo()` (TanStack Query, API
      client from an `ApiClientProvider` context). States: loading (accessible skeleton),
      success (version text), error ("Server unreachable" in a `role="status"` region).
- [ ] Every string comes from i18n. `en` and `de` are complete (completeness test from M1-14
      passes).
- [ ] Logic lives in hooks (`useSidebarState`, `useSearchShortcut`, `useSystemInfo`).
      Components only render. No raw hex values (lint).
- [ ] Tests (Vitest + Testing Library, fake API client through the provider): toggle flips
      `aria-expanded` and persists; Ctrl+K focuses search; version renders from the fake
      response; error state renders on `ApiError`; home empty state renders in German when
      the language is `de`.
- [ ] Gates: `pnpm -C web lint` (incl. jsx-a11y strict, dependency-cruiser) and
      `pnpm -C web test` (thresholds) pass.

## Notes
- Mobile bottom navigation, hero area, poster rows, activity indicator and profile switcher
  come in milestone 5. Do not stub them.
- Focus must be visible on every interactive element (token-based focus ring).
- Shared components (`Sidebar`, `TopBar`, `EmptyState`, `Skeleton`) go to `src/shared/ui`
  and must not import from `src/features/**`. The version footer is composed in
  `src/app/layout` from `features/system`.
- `staleTime: Infinity` is fine for system info. Retry once.

## Review carry-over from M1-14 (round 2)
- `web/eslint.config.js`: extend the hex-rule comment – text after `(` or `,` such as "Fixes (#123)" is also flagged and belongs in locale files.
- `web/apps/web/test/fontFamily.test.ts`: resolve the bare `@fontsource-variable/inter` specifier (as `main.tsx` imports it) instead of `/index.css`.
