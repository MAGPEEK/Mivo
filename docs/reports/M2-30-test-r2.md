# M2-30 test report, round 2

Result: PASS. Run in an isolated worktree at HEAD fd3295c. 60 files from `m2-30-files-r2.txt` were copied in (the round-1 list plus every new or changed file under `features/player/**`, plus ADR 0024). No api-client, Go or OpenAPI files were copied.

## Gates (worktree)
| Gate | Result |
|---|---|
| `pnpm -C web install --frozen-lockfile` | OK |
| `pnpm -C web lint` (eslint max-warnings=0, depcruise, prettier, tsc -b) | clean, 290 modules, no violations |
| `pnpm -C web test` | 82 files, 1203 tests passed. Coverage 100 % (statements 1749/1749, branches 995/995, functions 562/562, lines 1708/1708). |
| `pnpm -C web build` | OK. hls.js is still its own chunk: `hls-D2SbReJ5.js` 574 kB; main `index-Bp99ukbU.js` 481 kB. The chunk-size warning is pre-existing and comes from hls. |
| `pnpm -C web e2e` | 22/22 passed (player.spec.ts: direct play, HLS, full server and retry, overlay keyboard focus, axe with overlay open) |

No failures. No dependency on M2-32's uncommitted API changes showed up. The generated schema at HEAD was enough.

## Round-1 fixes
- K on a focused button: met. `playerKeys.ts` `isHandledByTarget` leaves only Space to a focused button. Letters are never left to the target. Covered by "leaves Space on a focused button..." in PlayerPage.controls.test.tsx:288 and playerKeys.test.ts.
- Controls do not auto-hide while keyboard focus is inside them: met. `useKeyboardFocusInControls` (focusin/focusout) feeds `useControlsVisibility(playingFreely && !keyboardFocusInControls)` in usePlayer.ts:117-118. Tested in useControlsVisibility.test.ts.
- No clamping to a borrowed session's duration: met. playbackEngine.seek.test.ts:284, 370 and 378 cover open-another-file, a seek during the new request, and a retry after failure. All pass.
- Scrubber listeners cleaned up on unmount: met. `useDragEndListeners` removes the pointerup and pointercancel listeners in an unmount effect (useScrubber.ts:22-45). Asserted at PlayerPage.controls.test.tsx:135 via removeEventListener.
- Play/pause before a stream sets the autoplay intent: met. `usePlayer.ts:129-142` calls `setAutoplay(!autoplay)`. Tested in PlayerPage.controls.test.tsx:320, usePlaybackSession.test.tsx:115 and the engine's play-intent tests (seek.test.ts:396).
- Toggle keys ignore key repeat: met. `playerKeyCommand` drops repeated events except the seek arrows. Tested at playerKeys.test.ts:63-70 and PlayerPage.controls.test.tsx:314.

## Acceptance criteria
- Controls, scrubber, time, volume, fullscreen, overlay toggle, 3 s auto-hide: met (controls tests, e2e player.spec).
- Keyboard map (Space/K, arrows, F, I, M, Escape, focus visible): met (playerKeys, controls and overlay tests).
- Stream-info overlay with mode colours, reasons, streams and encoder, and en/de key coverage: met (overlay tests, streamInfoLocales.test.ts, apiTypeChecks.test.ts).
- States: spinner, stream_not_ready, no_compatible_format, transcode_unavailable, session_limit_reached with retry: met (PlayerPage.test.tsx, e2e "full server and retry").
- Accessibility (labelled buttons, slider with aria-valuetext, labelled region, axe): met (component tests and e2e axe with overlay open).
- Tests and coverage thresholds: met, 100 %.
- Gates lint, test, build: met.

## History
- Round 1: review findings (see the review report).
- Round 2: PASS.

Worktree removal status is in the final message.
