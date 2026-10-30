# M2-07 test report: web routes and per-feature translations

Status: **PASS** (round 1)
Scope: web only (`pnpm -C web ...`). Go gates are out of scope for this task (no Go files changed
by M2-07). Wave 1 runs in parallel: `web/packages/api-client` and other Go/docs files changed
during the run (M2-01, others); they are reported, not judged.

## Gates

| Gate | Command | Result |
|---|---|---|
| Lint | `pnpm -C web lint` (ESLint `--max-warnings=0`, dependency-cruiser, Prettier `--check`, `tsc -b`) | PASS, run twice (start and end of the session); depcruise: "no dependency violations found (124 modules, 219 dependencies cruised)"; Prettier "All matched files use Prettier code style!" |
| Unit tests + coverage | `pnpm -C web test` | PASS: 27 files, 389 tests; Statements 100 % (332/332), Branches 100 % (153/153), Functions 100 % (106/106), Lines 100 % (329/329); "No files with missing coverage" (first run: 26 files, 384 tests, before the parallel M2-01 api-client tests landed) |
| Build | `pnpm -C web build` | PASS, vite built 235 modules, `index-Dw2rxgOF.js` 399.84 kB (gzip 127.77 kB) |
| E2E run 1 | `pnpm -C web e2e` | PASS, 10 passed (7.7 s) |
| E2E run 2 | `pnpm -C web e2e` | PASS, 10 passed (7.3 s) |
| Go / Docker / media | n/a | not part of this task, not run |

Playwright reports: `C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\playwright-report-1` and `...\playwright-report-2` (same directory).

Note: depcruise prints "You're running a globally installed dependency-cruiser" (pre-existing,
not a violation).

## Browser verification (embedded binary)

Built `go build -tags webui` into the scratchpad and started it with an isolated config
(`MIVO_CONFIG_DIR`, `MIVO_PATHS_*` relative, `MIVO_SERVER_HOST=127.0.0.1`, port 18477). Drove
it with Playwright Chromium (locales `en-US` and `de-DE`). No console or page errors in any
run. Server stopped afterwards.

| Path | Result (en) |
|---|---|
| `/libraries/x` | h1 "Library", nav=1, banner=1, search box=1 (inside the shell), text "The videos of library x will be listed here." |
| `/items/x` | h1 "Item details", nav=1, banner=1 (inside the shell) |
| `/items/x/play` | h1 "Player", nav=0, banner=0, search=0, one `main` (full-screen, no sidebar) |
| `/nope/at/all`, `/libraries`, `/items` | h1 "Page not found", nav=1, banner=1 (not-found inside the layout) |
| `/items/x/play?fileId=abc&t=42` | "Item x, file abc, from 42 s." URL kept |
| `/items/x/play?t=-5&fileId=` and `?t=abc` | "file primary file, from 0 s."; URL cleaned to `/items/x/play` |

German (`navigator.languages = de-DE`): `<html lang="de">`; `/libraries/x` "Bibliothek / Hier
erscheinen die Videos der Bibliothek x."; `/items/x` "Eintragsdetails"; `/items/x/play`
"Wiedergabe / Eintrag x, Datei Hauptdatei, ab 0 s."; not-found "Seite nicht gefunden"; home
"Startseite". All three new namespaces (library, item, player) translate; the shared
namespace keeps working. Interpolated route params render as text (a `<img onerror>` item id
produced 0 `img` elements).

## Type-safety claims (temporary edits, restored)

Each edit was made on a backed-up copy and restored from it; `sha256sum -c` confirmed the three
touched files are byte-identical again, `git status --short -- web` equals the status from
before the experiment, `pnpm -C web lint:types` passes afterwards. No `git add`/commit.

| Edit | Result |
|---|---|
| `library/locales/de.json` without `placeholder` | `tsc` FAILS: `features/library/locales/index.ts(9,51): error TS2741: Property '"placeholder"' is missing in type '{ heading: string; }' but required in type '{ heading: string; placeholder: string; }'` |
| `LibraryPage.tsx`: `t("headin")` | `tsc` FAILS: `LibraryPage.tsx(16,54): error TS2345: Argument of type '["headin"]' is not assignable ...` |
| `PlayerPage.tsx`: `t("placeholder2")` (key not in player namespace) | `tsc` FAILS: `PlayerPage.tsx(24,57): error TS2345 ... '["placeholder2"]'` |
| Extra key `extra` in `library/locales/de.json` | `tsc` passes (excess keys in a JSON import are not a type error, as the doc comment says), the locale test FAILS: `src/app/i18n.test.ts > locales > gives German exactly the keys of English in library: expected ['extra','heading','placeholder'] to deeply equal ['heading','placeholder']` |

So the claim holds as documented: missing German key and wrong `t()` key fail `tsc`; extra
keys are caught by the locale test.

## `validatePlayerSearch` with hostile input

Called directly (node type stripping) and through the real URL in the built app.

| Input | Result |
|---|---|
| `t`: -1, NaN, Infinity, `"42"`, `[1,2]`, `{}`, null, true, `10n` | dropped (`t: undefined`) |
| `t`: -0 | `0` (valid, `-0 >= 0`) |
| `t`: 1e308, 1e21, MAX_SAFE_INTEGER, URL `?t=99999999999999999999` | **accepted** unchanged (no upper bound); renders "from 100000000000000000000 s" |
| `t=1e400` in URL | parsed to non-finite, dropped |
| `fileId`: "", 42, array, object, null, true | dropped |
| `fileId`: `"../../etc/passwd"`, `"   "`, 100 000-char string, UUID | **accepted** (any non-empty string) |
| `__proto__` / `constructor.prototype` keys (JSON-parsed and own property), URL `?__proto__[t]=5&constructor[prototype][fileId]=z` | ignored, result `{}`; `({}).t` stays `undefined` (no prototype pollution) |
| unknown keys | not copied into the result |
| repeated `?t=1&t=2`, `?fileId[]=a&fileId[]=b` | dropped (array) |
| Result shape | always both keys `fileId` and `t` (undefined when dropped), as documented |

## Acceptance criteria

1. Routes: MET. `/libraries/$libraryId` and `/items/$itemId` render inside the shell (nav,
   banner, search present), `/items/$itemId/play` is full-screen via pathless layout route
   `full-screen` (nav=0, banner=0); search `{fileId?: string, t?: number >= 0}` is validated by
   `validatePlayerSearch`, invalid values dropped not thrown (browser and unit evidence above).
   Observation: the task text says `fileId?: string`, so non-UUID ids pass; there is no upper
   bound on `t`. Consumers (M2-27/M2-30) must treat both as untrusted.
2. Placeholder pages from each feature `index.ts` with a heading from their namespace: MET
   (`LibraryPage`, `ItemPage`, `PlayerPage`; headings Library / Item details / Player and the
   German equivalents verified in the browser).
3. Per-feature namespaces `library`, `item`, `player` with `locales/{en,de}.json`, registered
   once: MET. Registration is in `src/app/i18n.ts` (`featureNamespaces`), documented in
   `shared/i18n/resources.ts` and `i18n.ts`; `shared` does not import `features` (depcruise
   green). `src/app/i18n.test.ts` covers all four namespaces: language set, German keys equal
   English keys, non-empty strings, no product name; German typed as English shape (`tsc`
   evidence above). The old `shared/i18n/locales.test.ts` was replaced by this test.
4. Not-found and shell behaviour unchanged, e2e passes: MET. Not-found renders inside the
   layout (browser: nav=1, banner=1; router test "shows the not-found view in the app layout
   below a feature route"); e2e 10/10 twice, M1-15/M1-17 specs unchanged.
5. Tests: MET. `app/router.test.tsx` (memory history: both shell routes, player without
   sidebar/top bar/skip link, German, not-found, search pass-through and dropping, link
   building), `features/player/lib/playerSearch.test.ts` (13-row table), locale parity in
   `app/i18n.test.ts`.
6. Gates (lint, test with coverage thresholds, build, e2e): MET, see Gates table. Coverage
   stays at 100 %.

## Observations (not failures)

- No Playwright e2e spec covers the new routes (not required by the task; router tests and the
  manual browser check above do).
- Unrelated to M2-07, changed in the working tree by parallel agents during this run:
  `web/packages/api-client/**` (client.ts, schemas.ts, generated/schema.ts, tests), ADRs
  0022-0025 and Go files. All web gates were re-run at the end with these changes present and
  stayed green.

## History

- Round 1: PASS.
