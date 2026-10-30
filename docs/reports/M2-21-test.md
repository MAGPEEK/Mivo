# M2-21 test report: player-core timeline, seek planning, stream-info model

Status: PASS (round 1)
Scope: `web/packages/player-core` (verdict). Repo-wide web gates were also green at run time.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| ESLint (player-core) | `pnpm exec eslint packages/player-core --max-warnings=0` | exit 0 |
| Prettier (player-core) | `pnpm exec prettier --check packages/player-core` | "All matched files use Prettier code style!" |
| Layer rules | `pnpm -C web lint:deps` | "no dependency violations found (201 modules, 447 dependencies)" |
| Types | `pnpm -C web lint:types` (`tsc -b`) | exit 0, apps/web compiles |
| Vitest + coverage (player-core) | `vitest run packages/player-core --coverage` | 11 files, 278 tests passed; Statements 100 % (252/252), Branches 100 % (137/137), Functions 100 % (91/91), Lines 100 % (235/235) |
| Build | `pnpm -C web build` | exit 0 (apps/web vite build ok) |
| Full web lint (info) | `pnpm -C web lint` | exit 0 |
| Full web test (info) | `pnpm -C web test` | 48 files, 826 tests passed; per-package thresholds met (player-core >= 90 %, global >= 70 %) |
| E2E | not run | task names no E2E flows; skipped as instructed |

Go gates not applicable (no Go files in this task).

## Cross-checks

- Type compatibility: scratch `.ts` (deleted afterwards) asserted mutual assignability between
  player-core and generated api-client types for PlaybackSession, StreamDecision, PlaybackReason,
  PlaybackMode, PlaybackDelivery, PlaybackSessionState, StreamAction, MediaStreamKind and
  PlaybackReasonScope. `tsc` exit 0. A negative control (`PlaybackMode` vs `string`) failed with
  TS2344 as expected, so the check is not vacuous.
- Translation keys: a scratch test drove `streamInfo` with every known mode, reason, warning,
  stream kind, stream action, encoder kind (software and `_nvenc`) and both bit-rate units, plus an
  unknown value for each group. Every key in `streamInfoTranslationKeys` was produced, nothing
  outside the list was produced, and the list has no duplicates. Unknown mode, reason, warning,
  kind and action codes map to `streamInfo.<group>.unknown`. `constructor`, `__proto__`, `toString`
  and `hasOwnProperty` also map to `unknown`. An unknown mode gets the `textMuted` colour token.
  Reason and warning codes match `internal/domain/playback/plan.go`.
- `planSeek`, HLS session starting at 600 s, duration 3600 s, seekable 0..100 in element time:
  - 650 s, 600 s and 700 s (both ends) give `local` with time 50, 0 and 100.
  - 700.001 s, 599 s and 1800 s give `restart` at the target.
  - A target covered only by a buffered range gives `local`.
  - 3600 s, 3599.5 s and 99999 s give `restart` at 3599, which is 1 s before the end.
  - 3599 s and 3598 s are unchanged. -5 s and NaN give position 0.
  - A file shorter than the margin restarts at 0.
  - Direct play is always `local`, clamped to `[0, duration]`.
- `shouldRecreateSession`:
  - HTTP 404 and 410 with `concernsSession` give true; without it, false.
  - HTTP 503 (`stream_not_ready`), 500, 401, 403 and 200 give false.
  - Fatal `networkError` with `fragLoadError` or `fragLoadTimeOut` gives true.
  - Non-fatal, `mediaError`, `levelLoadError` and `manifestLoadError` without a 404/410 response give false.
- Timeline: `displayedPosition(hls, 30)` is 630 and `mediaTimeForPosition(hls, 630)` is 30. The
  displayed position clamps to the duration, NaN shows the start position, and a missing duration
  does not fail.
- Purity: grep of `src/{timeline,seek,streamInfo}` finds no DOM globals and no api-client import.
  `readTimeRanges` takes a `TimeRangesLike`. The only match is a doc comment. The player-core tsconfig
  has `lib: ["ES2023"]` and `types: []`, so DOM use would fail `tsc`. The only cross-package import is the
  type-only `playbackSession` types. Colour tokens are returned as names.

## Acceptance criteria

1. Timeline (displayed position clamped to `[0, duration]`, inverse for local seeks): met.
   `displayedPosition` / `mediaTimeForPosition` / `displayedRanges`; cross-check above and
   `timeline.test.ts`.
2. `planSeek` local vs restart, clamped positions: met (cross-check above, `planSeek.test.ts`).
3. `shouldRecreateSession`: met (cross-check above, `sessionRecovery.test.ts`). See observation 1.
4. `streamInfo(session)` view model without strings: met. The model has mode with a colour token
   name (`directPlay` / `directStream` / `transcode`), reason and warning keys with params, an
   unknown fallback, stream lines with M2-14 labels, a container change, an encoder and the source
   bit rate.
5. Types match the API schema structurally, no api-client import: met (mutual-assignability check, grep).
6. Table tests; player-core coverage >= 90 %: met (100 % on all four metrics).
7. Gates `pnpm -C web lint`, `test` and `build`: met (all exit 0).
8. Notes: ui-tokens is not imported, and only token names are returned. This is within "may be
   imported". `streamInfoTranslationKeys` is exported: met.

## Observations (non-blocking)

1. `sessionRecovery.ts:63-66` also returns true for a fatal hls.js network error with HTTP status
   404/410 on a non-segment request (for example `manifestLoadError` with a 404). AC 3 says "fatal
   network errors on segments; false otherwise". The extension is documented in the function's doc
   comment and fits the 404 or 410 session-gone case. The architect or reviewer should confirm it is
   intended, and if so mention it in the task or ADR 0024.
2. `streamInfo.ts` uses the token name `textMuted` for unknown modes. It is not one of the three
   names in the AC, but it is a real `colors` token in ui-tokens.
3. Four empty files are skipped by coverage. They are type-only modules.

## History

- Round 1: PASS.
