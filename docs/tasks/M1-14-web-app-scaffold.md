# M1-14 Web app scaffold: Vite, React, router, query, Tailwind, i18n
Owner: mivo-frontend
Depends on: M1-11, M1-12
Files/areas: `web/apps/web/**` (`package.json`, `index.html`, `vite.config.ts`,
`tsconfig*.json`, `src/main.tsx`, `src/styles.css`, `src/app/**`, `src/shared/i18n/**`,
`src/shared/lib/**`, `src/features/home/**` placeholder)

## Goal
A running React app with the agreed stack and folder structure, built straight into the Go
embed directory (ADR 0020), so the following tasks only add features.

## Acceptance criteria
- [ ] `pnpm -C web build` writes `index.html` and hashed files under `assets/` to
      `internal/adapters/http/webui/dist` (`emptyOutDir: true`).
- [ ] `pnpm -C web dev` serves the app on port 5173 and proxies `/api` to
      `http://localhost:8460`. The target can be overridden through an env variable read
      only in `vite.config.ts`.
- [ ] Stack: React 19, TanStack Router (code-based route tree with `/` → home placeholder and
      a not-found route), TanStack Query `QueryClientProvider`, Tailwind CSS v4 through
      `@tailwindcss/vite` importing `@mivo/ui-tokens/theme.css`.
- [ ] Inter is self-hosted (e.g. `@fontsource-variable/inter`). After a build,
      `Select-String -Path internal/adapters/http/webui/dist/index.html, internal/adapters/http/webui/dist/assets/*.css -Pattern 'https?://'`
      finds nothing (ADR 0010).
- [ ] `<title>` comes from `brand.productName` (e.g. through a Vite `transformIndexHtml`
      hook). `index.html` contains no literal product name.
- [ ] i18n: i18next + react-i18next. Resources in `src/shared/i18n/locales/en.json` and
      `de.json`. Detection from `navigator.languages`, fallback `en`. `<html lang>` follows
      the active language.
- [ ] Folders `src/app`, `src/features/home`, `src/shared/{ui,lib,i18n}` exist.
      dependency-cruiser rules from M1-11 pass.
- [ ] Tests (Vitest + Testing Library, jsdom): `de.json` has exactly the keys of `en.json`
      and no empty values; the router renders the home placeholder at `/` and the not-found
      view for an unknown path; the language helper sets `<html lang>`.
- [ ] Gates: `pnpm -C web lint` and `pnpm -C web test` (thresholds) pass.

## Notes
- Configure `outDir` with `fileURLToPath(new URL('../../../internal/adapters/http/webui/dist', import.meta.url))`.
  Vite warns about an outDir outside the root unless `emptyOutDir` is set explicitly.
- Code-based routing avoids a generated route tree in M1. Switching to file-based routing
  later needs an ADR note.
- Do not add Radix, hls.js or other libraries before a feature needs them.
- The root `package.json` already delegates `dev`, `build` and `e2e` (M1-11). Only add
  scripts to `web/apps/web/package.json`.
- Product name only through `@mivo/ui-tokens` brand (ADR 0002).
