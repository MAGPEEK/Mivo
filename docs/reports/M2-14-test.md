# M2-14 test report: player-core client profile and technical labels

Status: **PASS** (round 2)

Scope: web gates and the cross-checks named for the re-test. Go packages of wave 2 were not judged.
Scratch material (Go helper, bundles, probe scripts, raw browser results) lives in the session
scratchpad; nothing was added to the repo and no production code or test was touched.

## Round 2 changes under test

2000 ms per-probe `decodingInfo` timeout through the required `ClientProbes.startTimer`, memoised
queries; HLS delivery `mse` whenever MSE reports at least one HLS video codec, `native` only
without a usable MSE; codec limits are the intersection over the contexts in which the codec
plays (codec dropped if no variant is shared); a direct-play entry needs at least one playing
video codec; control characters stripped from the name before truncation; `resolutionLabel` by
frame size, empty for non-positive/non-finite; fixtures in Go order (`h264, vp9, av1`); ADR 0023
"Notes (M2-14)".

## Gates

| Gate | Command | Result |
|---|---|---|
| Lockfile | `pnpm -C web install --frozen-lockfile --offline` | exit 0, "Lockfile is up to date"; diff of `package.json` / `pnpm-lock.yaml` / `.dependency-cruiser.cjs` unchanged from round 1 (+1 / +3 / +12 lines) |
| ESLint | `pnpm -C web lint` (`eslint . --max-warnings=0`) | clean |
| dependency-cruiser | `lint:deps` | "no dependency violations found (137 modules, 243 dependencies cruised)" |
| Prettier (format check) | `lint:format` (`prettier --check .`) | "All matched files use Prettier code style!" |
| Types | `lint:types` (`tsc -b`) | clean, exit 0 |
| Unit tests | `pnpm -C web test -- --coverage` | 31 files, 531 tests passed |
| Coverage | same run | Statements 100 % (467/467), Branches 100 % (209/209), Functions 100 % (161/161), Lines 100 % (457/457), "49 files fully covered"; Vitest thresholds (70 % global, 90 % `packages/player-core/src/**`) met |
| Build | `pnpm -C web build` | exit 0 |
| E2E, run 1 | `pnpm -C web e2e` | 10 passed (9.3 s) |
| E2E, run 2 | `pnpm -C web e2e` | 10 passed (9.2 s) |

`depcruise` prints the "globally installed dependency-cruiser" environment warning (not a
violation; present since before this task).

## Acceptance criteria

1. **`detectClientProfile(probes, options)` with injected probes, no DOM globals: met.**
   `ClientProbes` now also requires `startTimer: (ms, onTimeout) => cancel`. Grep for
   `window|document|navigator|globalThis|setTimeout|clearTimeout|HTMLMediaElement` in non-test
   sources finds only comments (`probes.ts:3,5,45,51`, `fakeBrowsers.ts:14`); `tsconfig` lib is
   ES2023 without DOM.
2. **Probe tables: met** (unchanged; `probeTables.ts`, 10-bit variants, `codecLimits.ts`).
3. **`probably`/`maybe`, `decodingInfo.supported`, HLS delivery: met, with a deviation from the
   task text.** The task criterion says "`native` when `application/vnd.apple.mpegurl` plays, else
   `mse`". The implementation (as intended for round 2, documented in ADR 0023 "Notes (M2-14)")
   prefers `mse` whenever MSE plays at least one HLS video codec and uses `native` only otherwise
   (`probeHls`, `detectClientProfile.ts:136-155`). The task file `docs/tasks/M2-14-*.md` was not
   updated and still carries the old order; the architect should align it (see Findings).
4. **`ClientProfile` structurally identical to the API schema, no api-client import: met**
   (no `@mivo/api-client` reference in `packages/player-core/src`; round-1 type check unchanged,
   types file untouched in structure).
5. **Fake-probe tests for Chromium, Firefox, Safari, "no MSE"; Chromium and Firefox equal the
   reference profiles: met.** `detectClientProfile.test.ts:49-95` (`toEqual` against
   `fixtures/*.json`; also macOS Safari and iOS Safari cases). Strict cross-language comparison
   below.
6. **Technical labels: met** (`labels/*.test.ts`, exported from `index.ts`; extras 576p, 1440p, 8K
   and the `Math.round` fallback for heights below every tier are additive).
7. **player-core coverage >= 90 %: met** (100 %).
8. **Gates lint / test / build: met.**

## Cross-check 1: fixtures vs ADR 0025 and the Go reference (value AND order)

Go helper (scratchpad `gohelper/main.go`, run with `go run -overlay` so nothing is written into
the repo) maps `playbacktest.ChromiumDesktop()` / `FirefoxDesktop()` into
`apigen.ClientProfile` and marshals it with the generated JSON tags. A Node script
(`cmp.mjs`) canonicalises both sides (object keys sorted, **array order preserved**) and
compares them to `fixtures/*.json` (`.profile`):

```
chromium-desktop IDENTICAL (strict, array order incl.)
firefox-desktop IDENTICAL (strict, array order incl.)
```

Codec limits are now `h264, vp9, av1` in the fixtures as in Go. The ADR 0025 table (lines 22-28)
matches the same values: mp4 `h264 av1 vp9` / `aac mp3 opus flac`, webm `vp8 vp9 av1` /
`opus vorbis`, ogg (Firefox) `theora` / `vorbis opus flac`, HLS `mse` `h264 av1 vp9` /
`aac mp3 opus flac`, `h264` 8 bit with `Constrained Baseline, Baseline, Main, High`, `vp9`/`av1`
10 bit. Round-1 order finding is resolved.

## Cross-check 2: real browsers (round 2)

Method as in round 1: player-core bundled with rolldown (IIFE), served from `http://localhost:<port>/`,
`detectClientProfile` run with a `<video>` element, `MediaSource`, `navigator.mediaCapabilities`
and `startTimer = setTimeout` wrapper. Headless. Raw results: scratchpad
`browser-results-r2.json`, `firefox-result-r2.json`.

| Client | Version | Driver | Detection time |
|---|---|---|---|
| Microsoft Edge (`msedge`) | 156.0.0.0 | Playwright | 156 ms |
| Google Chrome (`chrome`) | 156.0.0.0 | Playwright | 71 ms |
| Playwright Chromium | 156.0.8078.4 | Playwright | 28 ms |
| Firefox (installed) | 159.0 | own page + `POST /result` | 143 ms |
| Playwright Firefox 157 | - | **cannot start** on this machine (`spawn UNKNOWN`, side-by-side configuration error; environment problem, unchanged from round 1) |

Differences to the reference profile (arrays compared order-insensitively):

| Client | HLS | Differences vs reference |
|---|---|---|
| Edge 156 | **`mse`** (round 1: `native`); video `h264 hevc av1 vp9`, audio `aac opus flac` | + `hevc` (mp4, HLS); HLS audio lacks `mp3`; + `mkv` (`h264 hevc av1 vp9 vp8` / `aac mp3 opus vorbis flac`); + `3gp` (`h264` / `aac`); **no `ogg`** (round 1: `ogg` with empty video); `h264` limit 10 bit with `High 10`; extra `hevc` limit (10 bit, `Main`, `Main 10`) |
| Chrome 156 | **`mse`**; same as Edge | same as Edge, `3gp` audio `[]` |
| Playwright Chromium 156 | **`mse`**; video `h264 av1 vp9`, audio `aac opus flac` | HLS audio lacks `mp3`; + `mkv` (`h264 av1 vp9 vp8` / `aac mp3 opus vorbis flac`); + `3gp` (`h264` / `[]`); **no `ogg`**; `h264` limit 10 bit with `High 10`; no `hevc` |
| Firefox 159 | `mse`; video `h264 hevc av1 vp9`, audio `aac opus flac` | + `hevc` (mp4, HLS); `mp3` missing in mp4 and HLS; + `mov` (`h264 hevc` / `aac`); + `mkv` (`h264 hevc av1 vp9 vp8` / `aac opus vorbis`); `ogg` absent (`canPlayType("video/ogg")` is `""`); `h264` limit equals reference; extra `hevc` limit |

Required outcomes: Chrome, Edge and Playwright Chromium report HLS `mse` (met); `ogg` is gone from
all Chromium profiles (met, `video/ogg` still answers `maybe` there but no Ogg video codec plays).
All four profiles were also confirmed in the raw JSON (`ogg in directPlay: false` for each).

HLS `mp3` missing in Chromium: checked directly. In msedge, chrome and bundled Chromium
`MediaSource.isTypeSupported('audio/mp4; codecs="mp4a.6B"')` (also `"mp3"` and `mp4a.69`) is
`false` and `decodingInfo(media-source)` is `supported: false`, while `canPlayType` says
`probably`. The profile is therefore correct for MSE delivery; the ADR 0025 reference lists `mp3`
in HLS for Chromium, so the server will have to transcode MP3 audio for HLS there (it does when
the profile says so).

## Cross-check 3: `decodingInfo` that never resolves

Run against the round-2 bundle in Node (`hang.cjs`), `startTimer` = `setTimeout` wrapper, a
`decodingInfo` returning `new Promise(() => {})`:

| Scenario | Time | Result |
|---|---|---|
| MSE yes, everything `probably`, HLS playable | **2010 ms** | `directPlay []`, `hls native` with empty codecs, `codecLimits []` |
| no MSE | **2009 ms** | same shape |
| MSE yes, HLS playlist not playable | **2011 ms** | `hls mse` with empty codecs |
| control: resolves immediately | 1 ms, 89/89 timers cancelled | full profile |
| `decodingInfo` throws synchronously | 2 ms | all unsupported |
| `decodingInfo` rejects | 3 ms | all unsupported |

All probes wait in parallel (89 timers), so the total stays at about one timeout. The native HLS
re-probe after an empty MSE result reuses the memoised `file` answers of the MP4 contexts and
adds no second wait. Name handling (bundle): 10 control chars followed by 70 `y` gives 64 `y`;
70 emoji give 64 code points; a name of only control characters gives `""`.

`resolutionLabel(height, width?)` spot checks: 1080x1920 (portrait) and 800x1920 (letterbox) give
`1080p`, 536x1280 gives `720p`, 3840x1608 gives `4K`, 4320x7680 gives `8K`; `0`, `-5`, `NaN`,
`Infinity` as height give `""`; an unusable width is ignored.

## Failures

None.

## Findings (non-blocking)

1. **Task text is stale (architect).** `docs/tasks/M2-14-player-core-client-profile.md` criterion 3
   still says HLS `native` first, else `mse`. Round 2 deliberately inverts this (ADR 0023 "Notes
   (M2-14)"). Update the criterion so the task and the code agree.
2. **Empty HLS profile keeps a delivery value.** When no HLS codec plays at all (for example
   hung `decodingInfo`), the profile can still say `delivery: "native"` or `"mse"` with empty codec
   lists. The decision engine must treat empty HLS codec lists as "no HLS" (M2-08/M2-09 owners).
3. **Reference profiles differ from real Chrome/Edge 156** beyond HLS: they play `mkv`, `3gp` and
   H.264 High 10 and (branded builds) HEVC; no `mp3` in MSE HLS. The fixtures are the
   conservative reference of ADR 0025, not a snapshot of current browsers. M2-27/M2-34 should
   not assume equality in real-browser E2E (`msedge` project).
4. Headless runs only; headed GPU decoders could add or remove codecs. Playwright Firefox could
   not be started here; the installed Firefox 159 was used instead.

## History

- Round 1: PASS. Gates green (31 files / 507 tests, coverage 100 %, lint, build, e2e twice).
  Fixtures equalled ADR 0025 and the Go values except for the order of the codec limits
  (`h264, av1, vp9` vs `h264, vp9, av1`). Real browsers (headless): Edge 156, Chrome 156,
  Playwright Chromium 156 reported HLS `native` (not `mse`) and an `ogg` entry with empty video
  codecs; Firefox 159 reported `mse`, `hevc` and `mkv` extras, no `ogg`/`mp3`; Playwright
  Firefox could not start. Type assignability to the generated api-client type and the negative
  dependency-cruiser checks passed.
- Round 2: PASS (this report). Fixtures now identical to the Go reference including order;
  Chrome/Edge/Chromium report HLS `mse` and no `ogg`; hung `decodingInfo` finishes in about 2 s.

REPORT
status: PASS
task: M2-14
files: docs/reports/M2-14-test.md
tests: pnpm -C web install --frozen-lockfile --offline (ok); pnpm -C web lint (eslint, depcruise 137 modules, prettier, tsc: clean); pnpm -C web test -- --coverage (31 files, 531 tests, 100 % stmts/branches/funcs/lines); pnpm -C web build (ok); pnpm -C web e2e x2 (10 passed each); strict JSON compare of both fixtures with Go-marshalled playbacktest profiles (identical, order incl.); real browsers Edge 156, Chrome 156, Chromium 156, Firefox 159 (Chromium family HLS mse, no ogg); never-resolving decodingInfo finishes in about 2.01 s
findings:
- Task file criterion 3 still states native-before-mse; code and ADR 0023 Notes (M2-14) now use mse-first. Architect should update the task text.
- Profile can carry delivery native/mse with empty HLS codec lists (all probes failed); the decision engine must treat that as no HLS.
- Real Chrome/Edge 156 differ from ADR 0025 references beyond HLS (mkv, 3gp, High 10, HEVC, no mp3 in MSE HLS); expected, not an M2-14 defect.
- Playwright Firefox cannot start on this machine (environment); installed Firefox 159 used.
next: Proceed to mivo-reviewer; ask mivo-architect to align the M2-14 task text (HLS delivery order).
