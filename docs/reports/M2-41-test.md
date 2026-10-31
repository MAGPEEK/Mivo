# M2-41 test report

Status: PASS (round 1)

Worktree base 1895eae, uncommitted changes. Changed files: 15 modified, 2 new tests
(`useScrubber.test.ts`, `playbackEngine.intent.test.ts`); all inside the task's allowed areas.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| Install | `pnpm -C web install --frozen-lockfile` | OK, lockfile up to date |
| Lint | `pnpm -C web lint` | exit 0 (ESLint, dependency-cruiser "no dependency violations found (304 modules)", Prettier clean, tsc -b clean) |
| Unit tests | `pnpm -C web test` | 84 files, 1220 tests passed; coverage 99.94 % stmts, 99.89 % branches, 100 % funcs, thresholds met (the only uncovered line, `useMediaControls.ts:29`, is in an unmodified file) |
| Build | `pnpm -C web build` | exit 0 (only the known >500 kB hls chunk warning) |
| Codegen | `pnpm -C web gen`, then `git status` | no generated file changed; still the same 17 changed paths |
| E2E run 1 | `pnpm -C web e2e` | 31 passed (chromium + branded), exit 0 |
| E2E run 2 | `pnpm -C web e2e` | 31 passed (chromium + branded), exit 0 |
| Go gates | not run | task touches only web files; FFmpeg present (used by e2e) |

## Acceptance criteria

1. useScrubber second pointerdown: MET. `useScrubber.ts` calls `remove.current?.()` before adding the new pair.
   `useScrubber.test.ts`: two pointerdowns, one pointerup gives `seek.mock.calls == [[70]]`; spies on
   add/removeEventListener show no pointerup/pointercancel listener left after the drag and after unmount;
   a second test covers unmount during a running drag.
2. Play/pause sets the engine intent: MET. `usePlayer.ts` `usePlayerMedia` always calls `setAutoplay(shownPaused)`;
   the engine's `setAutoplay` plays or pauses an attached element. `playbackEngine.intent.test.ts`: pause while
   playing, then (a) fatal hls.js network error recovery (session-2) and (b) `stream_not_ready` retry give no
   `play` after the `pause`, `autoplay: false`, and `setAutoplay(true)` then plays. Controls test
   "also sets the engine's intent" added. The engine does not follow element `pause` events; the why-comment is in
   `playbackEngine.ts` (setAutoplay doc). Existing tests green.
3. retry() respects intent: MET. `retry()` uses `this.autoplay`. Tests: a pause set in the error state is kept
   (no `play`), and the default intent plays. The old seek test that expected retry to reset the intent was
   rewritten to use `open()` ("plays every newly opened file"). This follows the new spec, not a weakened assertion.
4. Focus-visible on keydown: MET. `useKeyboardFocusInControls` adds a window `keydown` listener (removed on cleanup).
   Four tests with stubbed `matches(":focus-visible")`: not pinned after a click focus, pinned after keydown,
   not pinned when the keydown leaves no ring, not pinned when focus is outside the controls.
5. Locales: MET. en `{{sourceContainer}} not supported → {{targetContainer}}`, de
   `{{sourceContainer}} nicht unterstützt → {{targetContainer}}` (en.json:52, de.json:52). `streamInfoLocales.test.ts`
   renders reasons for transcode and remux sessions in both languages (no remux/transcode, case-insensitive) and
   scans every `streamInfo.reason.*` / `streamInfo.warning.*` text in both locales for mode words.
   Old assertions updated only in `PlayerPage.overlay.test.tsx` (2), `e2e/player.spec.ts:132`, `e2e/branded.spec.ts:52`.
6. Tests: MET (unit with thresholds, e2e twice in both projects).
7. Lint: MET.

## BACKLOG.md

`git diff` shows exactly two removed rows (the `player` M2-30 review r2 row and the `player locales` M2-34 review
row) and no other change.

## Notes

- Deviation (not a failure): `usePlayer.ts` replaces `playControl` with `usePlayerMedia`, and the element's own
  `togglePlay` is no longer used. The play button shows the element's `paused` state while playing and toggles via the
  engine intent. This matches the task's "simplest intent fix". ADR 0024 was not edited.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M2-41
files: docs/reports/M2-41-test.md
