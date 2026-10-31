# M2-34 test report (round 1)

Status: PASS

Environment: Windows 11, Node 24.22.1, pnpm 10.34.5, Go on PATH, FFmpeg 9.0.2 (WinGet), installed
Edge and Chrome. Clean worktree at HEAD (531f20b) plus only the 18 changed files under
`web/apps/web` (from `git status --porcelain -uall web/apps/web`).

## Gates

| Gate | Result |
| --- | --- |
| `pnpm -C web install --frozen-lockfile` | OK |
| `pnpm -C web lint` (eslint, depcruise, prettier, tsc -b) | exit 0, no violations (302 modules) |
| `pnpm -C web test` | 82 files, 1203 tests passed, coverage 100 % (thresholds met) |
| `pnpm -C web build` | OK (chunk-size warning only) |
| `pnpm -C web e2e` run 1 / 2 / 3 | 31 passed each (23.1 s / 25.4 s / 22.2 s), no retries, no flaky |
| E2E, branded channel forced absent (code patched in the worktree to `installed = false`, then restored) | 26 passed, 5 branded skipped, warning printed: `Skipping the branded project: The browser channel "msedge" is not installed. Install it or set MIVO_E2E_BRANDED_CHANNEL ...` |
| E2E with FFmpeg removed from PATH | global setup fails in 2 s with the `assertFFmpegAvailable` message (`spawnSync ffmpeg ENOENT` as cause), not a timeout |

Go gates and Docker: not applicable (only `web/apps/web` changed). The layer rule is not touched.
The absence run could not use the real environment: Windows resets `PROGRAMFILES`, so the
install check was forced in a throwaway copy; the original file is byte-identical to the main
checkout afterwards. A full run with an env-var-selected missing channel is not possible on this
machine because Edge is always present.

## Acceptance criteria

1. Global setup, corpus, FFmpeg check: MET. `globalSetup.ts` runs `assertFFmpegAvailable`
   first, starts the server with the media directory as library and runs
   `go run ./scripts/gen-testmedia -only <list> -out <root>/media` (6 cases, includes the four
   required). Missing FFmpeg: clear error, verified above.
2. Spec "library": MET. `library.spec.ts` shows the library heading, "Scanning..." (busy) then
   "Last scan: 6 added ...", and all titles in the grid. Axe on the library page also passes
   (`library.spec.ts:63`).
3. Spec "play open codec": MET. `playback.spec.ts:26` WebM, `mode direct_play`,
   `expectVideoPlays`, overlay label "Direct Play" with the direct-play colour assertion.
4. Spec "overlay without proprietary codecs": MET with deviation (see below).
   `playback.spec.ts:38` asserts HTTP 422, problem code `no_compatible_format` and the
   translated alert text.
5. Project `branded`: MET with deviations. MP4 Direct Play, FLV remux ("Direct Stream"), AVI
   Transcode each play, with mode and reason asserted (`FLV not supported -> remux to MP4`,
   `Video MPEG-4 not supported -> H.264`); seek test restarts the session (new id, start
   position within one keyframe interval of 50 %, old session deleted, position keeps
   advancing). Skip with a visible reason: verified.
6. `<root>/transcode/sessions` empty after the run: MET. The teardown runs
   `assertNoSessionsLeft` against the live server and would fail the run; it passed 4 times.
7. axe: MET. Library page, player (`playback.spec.ts`, each `branded.spec.ts` mode case,
   `player.spec.ts:141` with overlay open) report no serious/critical findings.
8. Gates: MET. `pnpm -C web e2e` green on Windows for both projects, lint green.

## Deviations from the task assumptions

- Codec emulation (criterion 4): ACCEPTED. The bundled Chromium decodes H.264 on this machine
  (as the task note from M2-14 anticipated), so a literal "Chromium has no H.264" check would
  fail. The spec denies `canPlayType`, `MediaSource.isTypeSupported` and
  `mediaCapabilities.decodingInfo` for avc/hvc/aac before the app loads and records via an
  annotation whether the real browser lacked H.264. The server-side behaviour (422
  `no_compatible_format`, translated message) is real; only the client profile is emulated.
  Intent (document the codec gap explicitly) is kept. Residual risk: the emulation covers the
  three probe APIs the profile detection uses; a future detection path would need adding.
- MKV is direct play on Chrome/Edge 156, FLV covers remux (criterion 5): ACCEPTED. The MKV
  test asserts Direct Play where the browser plays Matroska and remux otherwise (annotated),
  and the FLV case, which no browser plays, gives a deterministic remux with "Direct Stream"
  label and reason. Remux playback and its overlay are still covered. The task text says
  "MKV -> Remux"; this is a correct update to browser reality, not a weakening.
- Seek test truncates the playlist (criterion 5): ACCEPTED WITH NOTE. It denies Matroska so
  the 30 s MKV is remuxed, and rewrites the first session's playlist to list one segment
  with no end, so that 50 % is not buffered and the player must restart the session. That
  exercises the real restart path (new session on the server, old one deleted, real playback
  from the new start position) but does not test the server's progressive playlist, which a
  fully produced test clip never exposes. Fits the intent ("seek restarts the session and
  playback continues"). The added corpus clip `video-mkv-h264-aac-long` is the sixth clip.
- Corpus generated after the startup scan: ACCEPTED. It makes the library spec's scan the one
  that fills the library (an empty state, then "6 added"), which is what criterion 2 needs.
  Server and scan still run unmodified. The `library` project runs before all others via
  `dependencies`, meeting the task note.
- 6 clips instead of 4: ACCEPTED. The criteria say "at least"; extras are FLV and the long MKV.
  The criterion 2 wording "four titles" is satisfied as a subset (all six titles asserted).
  Note two clips share the title "h264 aac" (mkv, flv); the spec tells them apart by chip.

## Other observations (not failures)

- "Keep the existing M1 specs unchanged" is slightly deviated from: `shell.spec.ts` (+4 lines)
  now tabs to the library link before the footer (a library now exists), and
  `player.spec.ts` only has a comment update plus the new import. Both are required by the
  shared server now having a library; no assertions were weakened.
- Playwright's Firefox was not examined (outside this task's scope); the M2-14 note about a
  native-HLS profile is not triggered, the branded Chrome/Edge runs resolved to the expected
  modes.
- No failure occurred, so no `playwright-report` had to be kept.

## History

- Round 1: PASS.
