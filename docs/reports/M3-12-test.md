# M3-12 test report – web posters

Result: **PASS** (round 1)

## Gates

| Gate | Result |
|---|---|
| `pnpm -C web install --offline` | up to date |
| `pnpm -C web lint` (ESLint, dependency-cruiser 309 modules, Prettier, tsc -b) | clean, no violations |
| `pnpm -C web test` | 86 files, 1242 tests passed; coverage 99.88 % stmts / 99.8 % branches / 99.82 % funcs / 99.88 % lines (thresholds met) |
| `pnpm -C web build` | built (only the known >500 kB hls chunk warning) |
| `go vet -tags webui ./...` | clean |
| `golangci-lint run --build-tags webui` | 0 issues |
| `go test -race -tags webui ./internal/adapters/http/... ./cmd/...` | all ok |
| `pnpm -C web e2e` | 31 passed (chromium + branded), incl. axe specs; no regression |

Go domain coverage not re-run (no Go source changed; only web, docs/tasks/BACKLOG.md).

## Acceptance criteria

- Poster component: met. `shared/ui/Poster.tsx` – props imageRef/aspect/alt/sizes (plus optional icon/className); `<img>` with `src` (320) and `srcSet` via `imageUrl`/`imageSrcSet` widths 160, 320, 480, 640, 960; `loading="lazy"`, `decoding="async"`; `aspect-2/3` / `aspect-video` box; placeholder (token colours, icon) with `role="img"` + `aria-label=alt` when no ref or after `error`; retried when the ref changes.
- Grid cards: met. `ItemCard` uses `Poster` with `item.poster`; year shown; shows render "N season(s)" via `childCount` instead of chips; movies keep duration/resolution chip/unavailable badge. Shows are not marked unavailable. Grid `sizes` follows the breakpoints (`gridLayout.ts`).
- Item page poster: met. `ItemDetails` renders `Poster` with `item.poster`, `alt = item.title`; rest unchanged.
- i18n: met. `items.seasons_one/_other` in library `en.json` and `de.json` ("Staffel"/"Staffeln"); alt = item title; no hard-coded strings.
- BACKLOG: met. `web library` row deleted; `useLibraryScan.start` has a synchronous `starting` ref guard, reset in `onSettled` (tests: single POST for two starts, restart allowed after failure).
- Vitest: met. Poster (src/srcset/sizes, no ref, error event, aspect classes, re-try on new ref); LibraryPage tests for movie, placeholder and show cards; ItemPage with and without poster; axe tests still clean (LibraryPage axe case now includes a poster).
- Gates: met (table above).

## Accessibility

- Grid cards: the poster sits in the `MediaCard` wrapper with `aria-hidden="true"` (test asserts the image is inside an aria-hidden ancestor); the title link names the card and the details are `aria-describedby`.
- Focus: stretched-link focus ring and single tab stop unchanged; focus-on-mount tests still pass.
- Item page: poster `<img alt={title}>` / placeholder `role="img"` named by the title; axe clean.
- Motion/hover classes unchanged.

## Findings (non-blocking)

- Scope deviation: `web/packages/api-client/src/images.ts` and its test are touched (omit an empty `tag` in `imageUrl`) and the matching clause was removed from the BACKLOG "m3 wave 1" row. This is outside the task's listed areas but resolves a tracked M3-02 minor; reviewer should accept or reject it explicitly.
- Cosmetic: the `imageUrl` doc comment now has an over-long line (Prettier does not reflow comments).
- Cosmetic: `gridLayout.ts` starts with a stray comment line above the `itemGridClass` doc comment.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M3-12
files: docs/reports/M3-12-test.md
tests: pnpm -C web lint clean; pnpm -C web test 1242 passed, coverage 99.88 %; pnpm -C web build ok; go vet / golangci-lint / go test -race with -tags webui ok; pnpm -C web e2e 31 passed
findings:
- images.ts (api-client) change is outside the task's file list (empty tag omitted) – reviewer to decide
- cosmetic: long doc-comment line in images.ts, misplaced comment at top of gridLayout.ts
next: hand over to mivo-reviewer
