# M1-11 test report

Status: PASS (round 1)
Environment: Node v24.22.1, pnpm 10.34.5, Windows 11. Go gates intentionally not run (parallel M1-03).

## Gates (positive runs)

| Gate | Command | Result |
|---|---|---|
| Install | `pnpm -C web install --frozen-lockfile` | OK, lockfile up to date, 2 workspace projects |
| Lint | `pnpm -C web lint` | exit 0: ESLint `--max-warnings=0` clean; depcruise "no dependency violations found (4 modules, 4 dependencies)"; prettier "All matched files use Prettier code style!"; `tsc -b` clean |
| Test | `pnpm -C web test` | exit 0: 1 file, 8 tests passed; coverage Statements/Branches/Functions/Lines 100 % |
| Re-run after cleanup | lint + test | both exit 0 again |

Coverage thresholds (global 70 %, `packages/player-core/src/**` 90 %) are enforced; see negative check 6.
Node/Go-domain layer check and Go coverage are out of scope for this task.

## Acceptance criteria

- [x] Install with Node 24 / pnpm 10: OK. `packageManager: pnpm@10.34.5`, `engines.node: >=24`. Root and `@mivo/player-core` package.json both have `"license": "GPL-3.0-or-later"` and `"private": true`; names `@mivo/web-workspace`, `@mivo/player-core`.
- [x] Root scripts delegate: `lint` (eslint + depcruise + prettier --check + tsc -b), `test` (`vitest run`), `build`/`gen` (`pnpm -r --if-present run`), `dev`/`e2e` (`--filter ./apps/web --if-present`), `format`.
- [x] tsconfig.base.json: `strict`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, `verbatimModuleSyntax`, `moduleResolution: bundler`; project references in `tsconfig.json` (node + player-core, composite).
- [x] ESLint flat config: typescript-eslint strictTypeChecked + stylisticTypeChecked, react / react-hooks / jsx-a11y strict for tsx, import-x, sonarjs, `no-explicit-any` error, hex-colour `no-restricted-syntax` (ignored for `packages/ui-tokens/**`), `**/generated/**` and build output ignored, `max-lines-per-function` off for tests.
- [x] dependency-cruiser: all ADR 0019 rules present and each verified to fire (negative checks 2, 3, 7).
- [x] Vitest: `projects: ["packages/*", "apps/*"]`, v8 coverage `enabled: true`, thresholds 70 % global and 90 % for `packages/player-core/src/**` on all four metrics.
- [x] player-core: `ClientCapabilities` types and `detectContainerSupport(probe, candidates)` with injected `MediaTypeProbe`; tsconfig has `lib: ["ES2023"]` and `types: []` (no DOM, no Node), no React.
- [x] `pnpm -C web lint` and `pnpm -C web test` pass; coverage reported and thresholds met.
- [x] Negative checks shown below, each reverted.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Negative checks (temporary files under `web/`, all deleted; `git status` shows only the original untracked files)

1. `any` in `packages/player-core/src/tmpQaAny.ts`
   `1:30 error Unexpected any ... @typescript-eslint/no-explicit-any` (also 1:36).
2. Import cycle (`tmpQaCycleA.ts` <-> `tmpQaCycleB.ts`)
   ESLint: `1:1 error Dependency cycle detected import-x/no-cycle` on both files.
   depcruise: `error no-circular: tmpQaCycleA.ts -> tmpQaCycleB.ts -> tmpQaCycleA.ts`.
3. `import { useState } from "react"` in player-core
   depcruise: `error framework-free-packages-not-to-react: packages/player-core/src/tmpQaReact.ts -> react` (plus `not-to-unresolvable`, because react is not installed). ESLint also errors (`import-x/no-unresolved`).
4. Hex colour in a non-token file: `"#ff00aa"` and template literal `` `#abc` `` flagged with `no-restricted-syntax` ("Hex colour literals are only allowed in packages/ui-tokens"); the anchor string `"#section-2"` is correctly not flagged. In a `.tsx` file, `style={{ color: "#123456" }}` is also flagged, and `<img>` without alt triggers `jsx-a11y/alt-text`.
5. Prettier: badly formatted `tmpQaFmt.ts` gives `[warn] ... Code style issues found`, exit 1. `tsc -b`: `tmpQaType.ts(1,14): error TS2322`, exit 1.
6. Coverage: an untested file in player-core drops coverage to 50/40/75/60 and `pnpm test` exits 1 with `ERROR: Coverage for lines (60%) does not meet global threshold (70%)` and `... does not meet "packages/player-core/src/**" threshold (90%)` (both global and per-glob thresholds enforced).
7. App layer rules (temporary `apps/web/src/{app,shared,features/a,features/b}`, `apps/other`, `packages/api-client/src/generated`), all fired:
   - `shared-not-to-features-or-app: apps/web/src/shared/s.ts -> apps/web/src/features/b/index.ts`
   - `features-not-to-other-feature-internals: features/b/uses.ts -> features/a/internal.ts` (import of `features/a/index` was correctly allowed)
   - `apps-not-to-other-apps: apps/other/src/o.ts -> apps/web/src/app/app.ts`
   - `packages-not-to-apps: packages/player-core/src/tmpQaToApp.ts -> apps/web/src/app/app.ts`
   - `generated-client-only-through-entry: apps/web/src/app/usegen.ts -> packages/api-client/src/generated/g.ts`

## Observations (non-blocking)

- `depcruise` prints "You're running a globally installed dependency-cruiser" although the local `web/node_modules/.bin/depcruise` is used. This appears to be a false positive of the tool's detection under pnpm; it does not affect the result.
- `pnpm-workspace.yaml` sets `allowBuilds: unrs-resolver: false` with a rationale comment (lifecycle script deliberately off); install and all gates work without it.
- The react-module dependency-cruiser pattern was only exercised for the unresolved case (react is not installed yet); the pattern also covers `node_modules/.pnpm/...` paths for once apps/web adds react.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-11
files: docs/reports/M1-11-test.md
tests: `pnpm -C web install --frozen-lockfile` OK; `pnpm -C web lint` exit 0 (eslint, depcruise, prettier, tsc -b); `pnpm -C web test` exit 0 (8 tests, 100 % coverage, thresholds 70/90 enforced); negative checks for any, cycle (eslint + depcruise), react in player-core, hex colour, prettier, tsc, coverage threshold and all app-layer depcruise rules fail as expected
findings: none blocking; depcruise "globally installed" warning is a false positive
next: proceed to mivo-reviewer
