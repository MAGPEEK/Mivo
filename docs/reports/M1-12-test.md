# M1-12 test report: ui-tokens and brand

Result: **PASS** (round 1)

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| Lint (ESLint, dependency-cruiser, Prettier, tsc -b) | `pnpm -C web lint` | exit 0; "no dependency violations found (18 modules, 17 dependencies cruised)"; "All matched files use Prettier code style!" |
| Tests + coverage | `pnpm -C web test` | 6 files, 144 tests passed; Statements 100 % (55/55), Branches 100 % (16/16), Functions 100 % (8/8), Lines 100 % (54/54) |
| Go gates | skipped | out of scope (M1-03b runs in parallel) |
| E2E / media / Docker | skipped | task does not touch them |

Coverage thresholds of the web workspace are enforced by the Vitest config; the run exited 0.

## Acceptance criteria

- [x] colors.ts values: `src/colors.ts:10-32` has the nine ADR 0011 values exactly, plus directPlay `#3DD68C`, directStream `#4C9AFF`, transcode `#F5B544`. `colors.test.ts` asserts them.
- [x] typography/spacing/radii/shadows/index: `typography.ts` has the Inter stack and `web` + `tenFoot` scales; the smallest tenFoot size is 20 (`xs`), guarded by `minimumTenFootFontSize` and a test. `radii.ts` is 6/8/10. `index.ts` re-exports all modules.
- [x] brand.ts: `productName = "Mivo"`, `productId = "mivo"`, `repositoryUrl`. Grep for `Mivo` in `web/**/*.{ts,tsx}` finds 2 hits, both in `src/brand.ts` (lines 1 and 6, the comment and the constant). `brand.test.ts` does not contain the string.
- [x] theme.css: `@theme` block with `--color-*`, `--radius-sm/md/lg`, `--font-sans`, `--font-mono`, `--text-*`, `--spacing`, `--shadow-*`. `package.json` exports `./theme.css`.
- [x] Drift test (`test/drift.test.ts`): checks both directions plus the colour count. Negative checks run, see below.
- [x] Contrast test with exported `contrastRatio` (`src/contrast.ts:71`). I recomputed every pair independently with a separate Node script (WCAG formula):
  - text on background/surface/raised: 15.80 / 14.31 / 12.58
  - textMuted on background/surface/raised: 7.21 / 6.53 / **5.74** (smallest text pair, matches the claim)
  - onAccent on accent: **6.01** (matches the claim)
  - accent on background/surface: 6.79 / 6.15
  - directPlay 10.15 / 9.20, directStream 6.69 / 6.06, transcode 10.50 / 9.51 on background / surface
  - All thresholds (4.5 text, 3.0 non-text) are met. The proposed status colours were not changed.
- [x] No runtime dependencies: `package.json` has no `dependencies` field. `pnpm-lock.yaml` adds only `packages/ui-tokens: {}`.
- [x] Tests for `contrastRatio` edge cases: black/white = 21, identical = 1, 3- and 6-digit hex, case-insensitivity, 9 invalid inputs throw `InvalidColorError` with a `value` property.
- [x] Gates pass (table above).

Layer and hex rules: lint passed. The only files with hex literals are `colors.ts`, `theme.css` and two test files inside the ui-tokens package.

## Drift test negative checks

`theme.css` is untracked, so `git diff` shows nothing for it. I compared against a backup copy and the git blob hash instead.

1. Changed `--color-accent: #ff6b57` to `#ff6b58`. 2 tests failed:
   - `declares accent with the value from colors.ts`: `expected '#FF6B58' to be '#FF6B57'`
   - `has --color-accent in colors.ts with the same value`
2. Added `--color-extra: #123456`. 2 tests failed:
   - `has --color-extra in colors.ts with the same value`: `expected undefined to be '#123456'`
   - `declares exactly as many colours as colors.ts`: length 13 vs 12

Restore confirmed: `git hash-object theme.css` is `5b6b2cc72779521a380655819e1e5f75b20cccfc` before and after, and the backup compares identical (0 differences). A re-run afterwards gives 144 tests passed.

## Observations (not blocking)

- The drift test compares colour hex case-insensitively (theme.css is lowercase because of Prettier, colors.ts uppercase per ADR 0011). The test documents the reason. This is a deliberate and sound choice, not a weakened check.
- The header comment in `theme.css` line 8 is longer than 100 characters. Prettier does not reflow CSS comments, so lint passes. Cosmetic only.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-12
files: docs/reports/M1-12-test.md (temporary edits to web/packages/ui-tokens/src/theme.css were restored, hash identical)
tests: pnpm -C web lint exit 0; pnpm -C web test 144 passed, 100 % statements/branches/functions/lines; drift negative checks (changed value, extra colour) fail as intended
findings: none blocking; the minor observations are in the report
next: hand over to mivo-reviewer
