# M2-27 test report – web player engine (round 2)

Status: **PASS**

Tested: `HEAD 1f27e87` plus the 47 files of M2-27 (list in the orchestrator's `m2-27-files.txt`),
in a clean git worktree (the main tree holds uncommitted Go work of another task). The worktree
was removed afterwards (`git worktree remove`, then `Remove-Item -LiteralPath '\\?\…'` because of
"Filename too long", `git worktree prune`). No production code or test was edited.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `pnpm -C web install --frozen-lockfile` | PASS | `Done in 9.2s`, lockfile accepted |
| `pnpm -C web lint` (eslint --max-warnings=0, depcruise, prettier, tsc -b) | PASS | exit 0; `no dependency violations found (254 modules, 603 dependencies cruised)`; `All matched files use Prettier code style!` |
| `pnpm -C web test` (coverage on) | PASS | `Test Files 70 passed (70)`, `Tests 1052 passed (1052)`; Statements 100 % (1413/1413), Branches 100 % (810/810), Functions 100 % (446/446), Lines 100 % (1376/1376); `No files with missing coverage` |
| `pnpm -C web build` | PASS | `index-BZqZZsdj.js 457.07 kB`, separate `hls-D2SbReJ5.js 574.32 kB`; `index.html` only references `index-*.js` (no modulepreload of hls); index chunk contains `await import("./hls-D2SbReJ5.js")`; `bufferAppendError` occurs only in the hls chunk. Rolldown prints the >500 kB chunk warning for the hls chunk (expected, own chunk by design). |
| `pnpm -C web e2e` run 1 | PASS | `20 passed (7.1s)` incl. 3 `player.spec.ts` tests |
| `pnpm -C web e2e` run 2 | PASS | `20 passed (6.3s)` |
| `go vet -tags webui ./...` after web build | PASS | exit 0, no output |
| `go build -tags webui ./cmd/mivo` | PASS | binary built (21.1 MB), used for the browser runs |
| `docker build -f packaging/docker/Dockerfile -t mivo:qa27 .` | PASS | `DOCKEREXIT=0`; image `mivo:qa27` removed afterwards |
| `pnpm -C web audit --prod` (report only) | clean | `No known vulnerabilities found` |
| Flake proof, sequential | PASS | `features/player` + `packages/player-core` (29 files, 453 tests) 20x in a row: 20x `Tests 453 passed (453)`, `SEQ failures=0` |
| Flake proof, parallel load | PASS | 4 concurrent vitest runs of the same set while 20 busy-loop node processes saturated all 20 logical cores: 4x `29 passed`, `453 passed` |

Go domain/coverage gates are not affected by this task (web-only, Go tree at HEAD; the layer
rule is unchanged). `go vet -tags webui` is the Go gate the task names.

## Real browser runs

Script (kept outside the repo): `scratchpad/r2-27-browser.mjs`. It starts the worktree's
`-tags webui` binary on an isolated port, mocks the item and playback API plus the stream routes
with Playwright `context.route`, and serves real media bytes (the corpus MP4 with Range support;
an fMP4 HLS stream generated with FFmpeg from `testdata/media/video/h264_aac_long.mkv`:
libx264/aac, 15 segments of 2 s, `init.mp4`, written to a temp dir). The HLS playlist is served
as an EVENT playlist without `#EXT-X-ENDLIST`, growing by one segment per 2 s from 6 segments.
`--autoplay-policy=no-user-gesture-required` is set because the page has no play gesture in
automation. Results:

| Browser | Checks |
|---|---|
| Chrome 156.0.8078.76 (channel `chrome`), run 1 and run 2 | run 2: 55/55 ok (run 1: 53/55, see "Findings of the test script" below) |
| Edge 156.0.4314.50 (channel `msedge`) | 55/55 ok |
| Playwright Chromium headless shell 156.0.8078.4 | 55/55 ok |

Per requested item (identical result in all three browsers):

- (a) Direct play of the corpus `h264_aac_faststart.mp4`: `currentTime` advances
  (0.55 s after start in Chrome), status `playing`, the video `src` is the session's `streamUrl`
  (`/api/v1/playback/sessions/s1/file`), the POST profile has `hls.delivery = "mse"` (profile name
  `Chrome 156`) and no `startPositionSeconds`. Also `?t=5` on a 20 s `-stream_loop` copy of the
  clip: POST has `startPositionSeconds: 5`, playback starts at 5.0 and advances.
- (b) HLS via hls.js on the growing EVENT playlist: playback starts at `currentTime` 0.00 and
  reaches 2.48 s after 2.5 s; engine position equals the file position; the page loads exactly one
  `hls-*.js` script. With `?t=10` the POST asks for 10, the stream plays from its own clock 0
  (`ct 0.80`, displayed position `10.71`) and keeps advancing. Control experiment (plain hls.js 1.7.3
  in Chrome against the same 6-segment EVENT playlist): default config reaches `currentTime` 8.30
  after 2.5 s (it starts near the live edge, about 6 s), `startPosition: 0` reaches 2.41. So the
  `startPosition 0` setting is what keeps playback at the requested position.
- (c) A seek beyond the buffer (`engine.seek(24)`) creates exactly one session
  (`startPositionSeconds 24`, `replacesSessionId s1`); the new session plays at position 25.5;
  the old session is not deleted by the seek; a seek inside the seekable range creates no session.
  Five seeks within 935 ms (16, 18, 20, 22, 26) against a 400 ms create latency: maximum 1 create in
  flight over the whole run (server-side in-flight counter and start/end log both give 1); the
  chain is `s1@16 -> s2@20 -> s3@26` (each request replaces the session the previous one returned),
  the player ends in session `s4` at position 27.25 (target 26) and no intermediate session was
  stopped. Targets 18 and 22 were correctly dropped as non-newest waiting wishes.
- (d) Playlist 503 `stream_not_ready` with `Retry-After: 1` twice, then 200: exactly 3 playlist
  requests (hls.js itself did not retry), gaps 1013 ms and 1008 ms, status text "The server is
  preparing the stream…" shown meanwhile, then playback with advancing `currentTime`, one session.
- (e) `503 session_limit_reached` (Retry-After 3), `422 no_compatible_format`,
  `422 transcode_unavailable`, each in `en-US` and `de-DE`: translated text (en exact strings from
  `en.json`; de e.g. "Auf dem Server laufen gerade zu viele Wiedergaben. Versuche es gleich noch
  einmal."), the button ("Try again" / "Erneut versuchen") is visible, reachable with Tab, and a
  click creates a new session (2 POSTs total).
- (f) Leaving the page: with a client-side back navigation exactly one DELETE (`s1`) and no
  playlist or segment request later than 300 ms afterwards; with a full navigation
  (`about:blank`, `pagehide`) exactly one `keepalive` DELETE `/api/v1/playback/sessions/s1` seen by
  a recording reverse proxy in front of the server, and no further stream requests.
- axe-core on the player page, en and de, in the playing state and in the error state with the
  retry button: no violation at all (`violations(all)=none`, headings "Player" / "Wiedergabe").

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | `hls.js` exact version, dynamic `import()`, own chunk | met | `"hls.js": "1.7.3"` in `web/apps/web/package.json` (only dependency without `^`/`~` it adds); `hls-D2SbReJ5.js` separate, not in `index-*.js`, not preloaded; browser load of exactly one hls chunk only for HLS sessions (the direct-play e2e asserts none is fetched) |
| 2 | Profile once per page load with real probes; compile-time ClientProfile/session checks | met | `browserProfile.ts` (+test), `apiTypeChecks.ts` (`Assignable`/`SameKeys` both directions, `tsc -b` green in lint); real Chrome/Edge/shell POSTs carry a detected profile (`Chrome 156`, `hls.delivery mse`) |
| 3 | `usePlaybackSession`: POST, attach per delivery (file -> src, hls+mse -> hls.js, hls+native -> src), state, session, position | met | Real browser: file path (a), hls.js path (b); native path and states covered by `playbackEngine.delivery.test.ts` / `usePlaybackSession.test.tsx`; 100 % coverage |
| 4 | Seek: `planSeek` local vs new session with `startPositionSeconds` and `replacesSessionId`; old hls.js destroyed first | met | (c) in real browsers; seek tests `playbackEngine.seek.test.ts`; `startQueue` serialisation shown by (c) five-seek run |
| 5 | Recovery once, second failure is an error | met | `playbackEngine.recovery.test.ts`, `recoveryBudget.test.ts` (100 % coverage); not exercised in a real browser |
| 6 | Teardown: hls.js destroyed, DELETE with keepalive, also on `pagehide` | met | (f): one DELETE on client-side leave and on `pagehide`, stream requests stop; `playbackApi.ts` `stopSession` uses `keepalive: true` |
| 7 | Typed errors incl. `session_limit_reached` with retry and `stream_not_ready` auto-retry with `Retry-After` | met | (d) and (e) in real browsers, `playbackErrors.test.ts`, `retryAfter.test.ts` (cap 300 s) |
| 8 | Vitest with fake API and injected fake hls factory; coverage thresholds hold | met | 1052 tests, 100 % on all four metrics (thresholds 70 %, player-core 90 %) |
| 9 | Gates: frozen-lockfile install, lint, test, build, docker build | met | table above |
| Notes | Config: `enableWorker` true, bounded `maxBufferLength`; CSP note `worker-src blob:`; stream URL from the session only | met | `hlsJs.ts`: `enableWorker: true`, `maxBufferLength 30`, `maxMaxBufferLength 60`, comment about `worker-src blob:`; engine uses `session.streamUrl` (the browser requests `/api/v1/playback/sessions/<id>/file|hls/…` exactly as the mock returned them) |
| Orchestrator notes | `hlsJs.test.ts` asserts `ErrorTypes.NETWORK_ERROR === "networkError"`, FRAG_LOAD_ERROR/TIMEOUT literals, `ErrorData` assignability; unknown delivery refused; `axe-core` devDependency and simplified `src/test/axe.ts`; lockfile owned by this task | met | `hlsJs.test.ts:33-35`, `apiTypeChecks.ts`, `package.json` has `"axe-core": "^4.14.0"`, lint/test green |

Round 2 additions verified: StartQueue (c), startPosition 0 (b + control), manifest `errorRetry`
0 (d, three requests in total with engine pacing), seek before the first session is applied
(unit tests), single `recoverMediaError` per position (unit tests), `restartPosition` exported
from `@mivo/player-core` (`index.ts:72`), Retry-After cap 300 s (`retryAfter.ts:11`), t=0 for
unknown durations (unit tests, 100 % coverage).

## Findings

None blocking. Observations for the orchestrator:

1. The player page does not wire `seek` to any control yet (`PlayerPage.tsx` destructures only
   `attachVideo, status, waitingForStream, error, retry`; the native `<video controls>` seek bar
   only updates the displayed position, it does not restart sessions). The browser seek checks
   therefore called the real engine's `seek` found through the React fiber of the built app.
   Seek UI belongs to M2-30; until then a user dragging the native seek bar beyond the buffer
   does not create a new session.
2. The `hls` chunk is 574 kB (178.9 kB gzip), flagged by the Rolldown size warning; intended
   (lazy chunk), only informational.
3. `depcruise` prints "globally installed dependency-cruiser" in `pnpm lint`; pre-existing, exit 0.
4. A keepalive DELETE issued on `pagehide` is not visible to Playwright `page.route`/`context.route`
   (the first Chrome run showed 0 deletes for that reason). With a real network observer the
   request arrives; this is a tooling limit and not a product defect.

### Findings of the test script (not product defects)

- Run 1 on Chrome reported two failures that were errors of the QA script: an over-strict
  expectation of at most 2 follow-up creates in the five-seek case (3 are correct with a 400 ms
  create latency: the chain stays serial and ends at the last target), and the `pagehide` DELETE
  observed through Playwright routing (see finding 4). Both checks were corrected and rerun; run 2
  and the two other browsers are 55/55.

## History

- Round 1: no report file in the repository (verdict given to the orchestrator directly).
- Round 2: PASS, as above.

REPORT
status: PASS
task: M2-27
files: docs/reports/M2-27-test.md
tests: clean worktree HEAD 1f27e87 + 47 files; pnpm install --frozen-lockfile ok; pnpm -C web lint ok; pnpm -C web test 70 files / 1052 tests, coverage 100 % (stmts/branches/funcs/lines); build ok (hls-D2SbReJ5.js separate from index-BZqZZsdj.js); pnpm -C web e2e 20/20 twice; go vet -tags webui and go build -tags webui ok; docker build ok (image removed); pnpm audit --prod clean; flake proof 20x sequential + 4x parallel under full CPU load, 453/453 each; real browser (Chrome 156, Edge 156, Chromium shell 156) 55/55 each for direct play, hls.js on growing EVENT playlist (starts at 0), seek restart/replacesSessionId, five rapid seeks (max 1 in flight, chained), 503 Retry-After, error mapping en/de with reachable retry, one DELETE on leave and pagehide, axe en/de clean
findings:
- none blocking
- seek is not wired to any page control yet (M2-30); engine seek tested via the built app's React fiber
- hls chunk 574 kB (informational)
- round-1 QA script errors (seek-chain bound, pagehide DELETE visibility in Playwright routing) corrected, not product defects
next: orchestrator may proceed to mivo-reviewer for M2-27
