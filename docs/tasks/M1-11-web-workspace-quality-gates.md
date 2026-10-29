# M1-11 pnpm workspace, web quality gates and player-core skeleton
Owner: mivo-frontend
Depends on: none
Files/areas: `web/package.json`, `web/pnpm-workspace.yaml`, `web/pnpm-lock.yaml`,
`web/tsconfig.base.json`, `web/tsconfig.json`, `web/eslint.config.js`, `web/.prettierrc.json`,
`web/.prettierignore`, `web/.dependency-cruiser.cjs`, `web/vitest.config.ts`,
`web/packages/player-core/**`

## Goal
The web monorepo exists with all quality gates from ADR 0019 wired into `pnpm -C web lint`
and `pnpm -C web test`. A first framework-free package (`player-core`) proves that the gates
and coverage thresholds work.

## Acceptance criteria
- [ ] `pnpm -C web install` succeeds with Node 24 and pnpm 10. `packageManager` pins the pnpm
      version. `engines.node` is `>=24`. Every `package.json` has
      `"license": "GPL-3.0-or-later"` and `"private": true`. Package names use the `@mivo/*`
      scope (rename checklist, ADR 0002).
- [ ] Root scripts **delegate**, so later tasks never edit the root `package.json`: `lint`
      (ESLint + dependency-cruiser + `prettier --check` + `tsc -b`), `test` (`vitest run`),
      `build` (`pnpm -r --if-present build`), `gen` (`pnpm -r --if-present gen`), `dev` and
      `e2e` (filtered to `apps/web`, `--if-present`), `format`.
- [ ] TypeScript: `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`,
      `verbatimModuleSyntax`, `moduleResolution: bundler`, with project references.
- [ ] ESLint flat config as in ADR 0019, including React/jsx-a11y rules for `*.tsx` (ready
      for `apps/web`), `no-explicit-any` as error, and a `no-restricted-syntax` rule
      forbidding hex colour literals outside `packages/ui-tokens`. Generated folders
      (`**/generated/**`) and build output are ignored. Test files may exceed
      `max-lines-per-function`.
- [ ] dependency-cruiser config contains every rule listed in ADR 0019, including the
      `apps/web/src/{app,features,shared}` rules (they apply once those folders exist).
- [ ] Vitest: root config with `projects` for `packages/*` and `apps/*`. v8 coverage
      **enabled by default**. Thresholds: global 70 % (lines, branches, functions,
      statements) and `packages/player-core/src/**` 90 %.
- [ ] `packages/player-core`: framework-free TypeScript exporting `ClientCapabilities` types
      and `detectContainerSupport(probe, candidates)`, which asks an injected probe
      (`canPlayType`-like interface) and maps `''|'maybe'|'probably'` to a support level.
      No React, no direct `document` or `window` access.
- [ ] `pnpm -C web lint` and `pnpm -C web test` pass. Coverage is reported and thresholds are
      met.
- [ ] The REPORT shows, each temporary and reverted: an `any` → ESLint error; an import
      cycle → import-x and dependency-cruiser errors; `import 'react'` in player-core →
      dependency-cruiser error; a hex colour in a non-token file → ESLint error.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Plugins: `typescript-eslint`, `eslint-plugin-react`, `eslint-plugin-react-hooks`,
  `eslint-plugin-jsx-a11y`, `eslint-plugin-import-x` (flat-config native),
  `eslint-plugin-sonarjs`.
- pnpm 10 does not run dependency lifecycle scripts by default. If a tool needs one (e.g.
  esbuild), add it to `onlyBuiltDependencies` in `pnpm-workspace.yaml` deliberately and
  explain why in the REPORT.
- Prettier: LF line endings, width 100. Keep other options default. Ignore generated code and
  build output.
- `web/pnpm-lock.yaml` is shared by all web tasks. It changes only through `pnpm install`.
  On conflicts, regenerate it, never hand-merge.
