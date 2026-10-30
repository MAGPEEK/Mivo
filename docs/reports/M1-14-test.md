# M1-14 test report

Status: PASS (round 2; round 1 sections below stay as historical record)
Scope: web gates only (Go gates skipped on request, parallel Go work running; no Docker, no media, no E2E in this task).

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| Install | `pnpm -C web install --frozen-lockfile` | OK, "Lockfile is up to date", exit 0 |
| Lint | `pnpm -C web lint` (eslint --max-warnings=0, depcruise, prettier --check, tsc -b) | exit 0; "no dependency violations found (47 modules, 63 dependencies cruised)"; "All matched files use Prettier code style!" |
| Test | `pnpm -C web test` | 11 files, **183 passed (183)**; coverage Statements 100 % (93/93), Branches 100 % (24/24), Functions 100 % (24/24), Lines 100 % (90/90); thresholds (70 % global, 90 % player-core) met |
| Build | `pnpm -C web build` | exit 0; 187 modules; output in `internal/adapters/http/webui/dist` |

## Build output checks

- `dist/index.html` plus `dist/assets/index-CtBk9LEg.js`, `index-tMKBHoFk.css` and 7 `inter-*-wght-normal-*.woff2` (hashed names, Inter self-hosted).
- `<title>Mivo</title>` present in dist/index.html; source `web/apps/web/index.html` has no product name (only a comment mentioning the brand module). `Grep Mivo` over `web/apps/web` (excluding node_modules) finds nothing.
- `Select-String -Pattern 'https?://'` over `dist/index.html` and `dist/assets/*.css`: no matches (ADR 0010).
- The built JS contains the generic strings `http://` / `https://` (library code); not covered by the criterion.
- `dist` deleted afterwards. The now-empty `internal/adapters/http/` directories created by the build were removed as well. `git status` shows no leftover. Note: `dist` is still not gitignored (git check-ignore returned no match); M1-16 must handle this.

## Dev server (extra check)

`pnpm -C web dev` with `MIVO_DEV_API_TARGET=http://localhost:18460` and a stub backend: `http://localhost:5173/` served index.html with `<title>Mivo</title>`; `GET /api/ping` returned `backend-ok /api/ping` (proxy works, env override works). The default target `http://localhost:8460` is set in `vite.config.ts` (`defaultApiTarget`); I did not test it against a live server (port used by Go work). Dev server and stub backend were stopped afterwards.

## Acceptance criteria

| Criterion | Status | Evidence |
| --- | --- | --- |
| Build writes hashed assets to webui/dist, `emptyOutDir: true` | met | build output above; `vite.config.ts` build.outDir/emptyOutDir |
| Dev on 5173, `/api` proxy to 8460, env override only in vite.config.ts | met | dev check above; `strictPort: true`; variable read only via `loadEnv` in vite.config.ts |
| React 19, TanStack Router code-based tree (`/` and not-found), QueryClientProvider, Tailwind v4 via `@tailwindcss/vite` importing `@mivo/ui-tokens/theme.css` | met | package.json (react ^19.3.1), `src/app/router.ts`, `src/styles.css` |
| Inter self-hosted, no `https?://` in html/css | met | `@fontsource-variable/inter` imported in main.tsx; Select-String empty |
| Title from `brand.productName`, no literal name in index.html | met | `brandTitle` transformIndexHtml plugin; dist title is "Mivo" |
| i18n (i18next + react-i18next), en/de JSON, `navigator.languages` detection, fallback en, `<html lang>` follows language | met | `createI18n.ts`, `documentLanguage.ts`; App.test.tsx "re-renders and updates <html lang>" passes |
| Folders app, features/home, shared/{ui,lib,i18n}; depcruise rules pass | met | folders present (`shared/ui/.gitkeep`); depcruise clean |
| Tests: locale key parity and non-empty values; router `/` and not-found; language helper sets `<html lang>` | met | `locales.test.ts`, `App.test.tsx`, `documentLanguage.test.tsx` all green within 183 |
| Gates lint and test pass | met | see Gates |

## Review carry-over M1-11

| Item | Status | Evidence |
| --- | --- | --- |
| react-hooks rules on `apps/**/*.{ts,tsx}` | met | temp `.ts` file with conditional `useState` in `apps/web/src/shared/lib`: `4:12 error React Hook "useState" is called conditionally ... react-hooks/rules-of-hooks` |
| Generalised React pattern in dependency-cruiser | met | temp `packages/player-core/src` import of `@radix-ui/react-slot`: `error framework-free-packages-not-to-react: packages/player-core/src/tmpNeg4.ts -> @radix-ui/react-slot` (also `not-to-unresolvable`, package is not installed) |
| Left-anchored hex check | met | `"Episode #123"` and `"/library#add"` not flagged; `"#ff6b57"`, `"bg-[#ff6b57]"`, `"color: #fff"` flagged (see note) |
| Coverage excludes `**/*.gen.ts` and app entry | met | `web/vitest.config.ts` exclude list has `**/*.gen.ts` and `apps/*/src/main.tsx` |
| ESLint `settings.react.version` in sync with installed react | met | `eslint.config.js` reads `react/package.json` from `apps/web` |

## Review carry-over M1-12

| Item | Status | Evidence |
| --- | --- | --- |
| `packages/ui-tokens` in the framework-free rule | met | temp `packages/ui-tokens/src` import of `react`: `error framework-free-packages-not-to-react: packages/ui-tokens/src/tmpNeg5.ts -> react` |
| ESLint product-name guard (case-sensitive, string and template), except `brand.ts` | met | temp files: `"Mivo"` and `` `Welcome to Mivo` `` flagged in apps (`1:21`, `2:20`) and in `packages/ui-tokens/src` (`1:18`); `brand.ts` itself lints clean |
| ui-tokens tests: `onAccent` on `accentHover`, status/accent colours on `surfaceRaised` | met | `colors.test.ts:49` pairs `onAccent` with `accent` and `accentHover`; `surfaceRaised` in text pairs (line 48) and in the accent/status group (line 61) |
| theme.css comment wrapped, drift scope stated, `white`/`black` note | met | `theme.css:1-17` wrapped; states both-direction check over every declaration, which `drift.test.ts` "declares exactly the variables of the TypeScript tokens" implements for all namespaces; `white`/`black` note at lines 13-14 |

## Notes (non-blocking)

1. A bare string `"#add"` IS flagged by the hex rule (`^#add` is a valid 3-digit colour and indistinguishable from `"#fff"`). Only fragments inside a longer string (`"/library#add"`) and text such as `"Episode #123"` are allowed. The carry-over wording lists `"#add"` as not flagged; the implementation comment documents the narrower behaviour. This is logically unavoidable if `"#fff"` must be flagged, so I did not count it as a failure. The reviewer may want to confirm.
2. `lint:deps` prints a "globally installed dependency-cruiser" warning although `web/node_modules/.bin/depcruise` exists and the dev dependency is declared; cosmetic (probably the CLI's own heuristic with pnpm). Not a violation.
3. `dist` is not gitignored yet (M1-16).

All temporary negative-check files, the stub backend and the dev server were removed or stopped. Nothing was staged.

## History

- Round 1: PASS.
- Round 2: PASS (see section below).

## Round 2 (after review)

Scope: web gates only; no Go gates, no Go files touched, nothing staged.

### Gates

| Gate | Command | Result |
| --- | --- | --- |
| Install | `pnpm -C web install --frozen-lockfile` | "Lockfile is up to date", exit 0 |
| Lint | `pnpm -C web lint` | exit 0; "no dependency violations found (49 modules, 66 dependencies cruised)"; Prettier clean; `tsc -b` clean (new `web/apps/web/test` project is referenced from `web/tsconfig.json`) |
| Test | `pnpm -C web test` | 12 files, **184 passed (184)**; coverage 100 % (Statements 93/93, Branches 24/24, Functions 24/24, Lines 90/90); thresholds met |
| Build | `pnpm -C web build` | exit 0; 187 modules; same hashed assets as round 1 (`index-CtBk9LEg.js`, `index-tMKBHoFk.css`, 7 Inter woff2) |

### Build output

- `dist/index.html` contains `<title>Mivo</title>`; `Select-String 'https?://'` over `dist/index.html` and `dist/assets/*.css`: no matches.
- `git check-ignore -v internal/adapters/http/webui/dist` -> `.gitignore:15:/internal/adapters/http/webui/dist/` (round-1 note resolved). The build output does not show up in `git status`.
- `dist` (and the then-empty `webui` directory) deleted afterwards. `internal/adapters/http/apigen/*` in git status belongs to the parallel Go work, not to this task.

### Hex rule negative/positive checks (temporary files in `apps/web/src/shared/lib`, deleted afterwards)

Flagged (`no-restricted-syntax`, "Hex colour literals are only allowed in packages/ui-tokens"):

- `"shadow-[0_1px_#fff]"` (1:18)
- `"border-[1px_solid_#f00]"` (2:18)
- `"1px solid #ff6b57"` (3:18)
- extra: `"x_#fff_inset"` (4:18), i.e. `_` as delimiter before and after a colour

Not flagged (eslint reported no problem for the file): `"Episode #123"`, `"#section-2"`, `"/library#add"`, and `"1px solid #fff"` (3-digit after whitespace, the documented trade-off in `eslint.config.js:42-44`).

### Font family test

`web/apps/web/test/fontFamily.test.ts` passes with the real value. After temporarily changing `selfHostedSansFamily` in `web/packages/ui-tokens/src/typography.ts` from "Inter Variable" to "Inter Renamed": `fontFamily.test.ts:22` fails (`- "Inter Renamed" / + "Inter Variable"`). File restored from a byte copy; SHA-256 of the file and the `git diff` of that file are identical to the state before the experiment; the test passes again afterwards.

### Other round-2 changes

- `vite.config.ts` `resolveApiTarget`: verified via `vite.resolveConfig` (a temporary script, deleted) reading `server.proxy['/api'].target`: env unset -> `http://localhost:8460`; `""` -> `http://localhost:8460`; `"   "` -> `http://localhost:8460`; `http://localhost:18460` -> same; `" http://x:1 "` -> `http://x:1` (trimmed). The function is not unit tested (config file, not in coverage); behaviour verified manually only.
- `App.test.tsx`: no `toBeDefined` remains in `web/apps/web/src` (assertions use `toBe`, `toContain`, `toBeNull`, `getAttribute`).
- `web/tsconfig.json` references and `apps/web/test/tsconfig.json` (Node types only for tests outside `src`): covered by `tsc -b` and the ESLint project service, both clean.

All temporary files and the experiment were reverted; `git diff --cached` is empty.
REPORT
status: PASS
task: M1-14
files: docs/reports/M1-14-test.md
tests: pnpm -C web install --frozen-lockfile OK; pnpm -C web lint OK (49 modules, prettier, tsc -b); pnpm -C web test 184/184, coverage 100 %; pnpm -C web build OK (title Mivo, no http(s):// in html/css, dist gitignored, deleted afterwards)
findings:
- Hex rule: shadow-[0_1px_#fff], border-[1px_solid_#f00], "1px solid #ff6b57" flagged; "Episode #123", "#section-2", "/library#add" not flagged
- fontFamily.test fails when selfHostedSansFamily is changed (typography.ts restored, hash and git diff identical)
- resolveApiTarget: unset, empty and blank env fall back to http://localhost:8460 (manual check, no unit test)
- Note: "1px solid #fff" (3 digits after whitespace) stays unflagged by design
next: orchestrator may proceed to mivo-reviewer round 2 for M1-14
