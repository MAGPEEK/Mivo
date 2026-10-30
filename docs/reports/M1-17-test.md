# M1-17 test report – Playwright E2E for the walking skeleton

Status: **PASS** (round 2; round 1 was PASS as well)

Scope: web gates and E2E. Go gates were not run: M1-17 changes no Go code (the uncommitted M1-20
files are in the tree and are built into the E2E binary, as expected).

## Gates

| Gate | Command | Result |
|---|---|---|
| Install | `pnpm -C web install --frozen-lockfile` | OK, "Lockfile is up to date" |
| Lint | `pnpm -C web lint` | exit 0: eslint `--max-warnings=0`, dependency-cruiser "no dependency violations found (100 modules, 175 dependencies)", prettier "All matched files use Prettier code style", `tsc -b` clean |
| Unit tests | `pnpm -C web test -- --coverage` | 23 files, **280 passed (280)**; coverage 100 % statements / branches / functions / lines (248/248, 118/118, 72/72, 245/245) |
| Build | `pnpm -C web build` | exit 0, `vite build` wrote `internal/adapters/http/webui/dist` |
| E2E run 1 | `pnpm -C web e2e` | **8 passed (5.5 s)**, wall 7.2 s including build |
| E2E run 2 | `pnpm -C web e2e` | 8 passed (5.6 s) |
| E2E run 3 | `pnpm -C web e2e` | 8 passed (5.4 s) |
| Flakiness | `playwright test --repeat-each=3` | 24 passed (8.3 s), no flaky or retried tests |

Note: `lint:deps` prints a harmless warning that dependency-cruiser is installed globally. It does not affect the result.

## E2E tests (all Chromium, all green)
1. renders the shell with navigation, search, home heading and empty state
2. shows the version the system info API reports in the sidebar footer
3. focuses the search box with Control+K
4. collapses and expands the sidebar and keeps the state across reloads
5. serves the app for a deep link and shows the not-found view (status 200)
6. home has no serious or critical axe violations, sidebar expanded and collapsed (2 tests)
7. keeps an unreachable server visible in the collapsed sidebar (extra, covers the carry-over)

## Isolation and cleanup (after 5 e2e invocations in total)
- `Get-Process mivo*`: none left.
- `%TEMP%\mivo*`: none left (the server log showed `config_dir=...\Temp\mivo-e2e-RgG0Po\config`, so the run used the temp dir, which teardown removed).
- `%LOCALAPPDATA%\Mivo`: absent before and absent after. No `*ivo*` directory under LOCALAPPDATA, APPDATA, ProgramData or USERPROFILE before or after.
- `e2e/support/server.ts` builds with `go build -tags webui` into the temp dir and listens on 127.0.0.1 with a free port (log: `address=127.0.0.1:51463`). It strips every inherited `MIVO_*` variable and redirects LOCALAPPDATA, APPDATA, USERPROFILE, HOME and XDG_* plus the four `MIVO_*` dirs into the temp dir. It checks for Go on PATH and for the web build and fails with a clear message. It waits for `/api/v1/health`, and teardown kills the process and removes the dir with retries.
- `test-results/` and `playwright-report/` are git-ignored (`.gitignore:17-18`); `webui/dist` is ignored too (`.gitignore:15`).

## Acceptance criteria (with the orchestrator decision applied)
- [x] `pnpm -C web e2e` builds the web app, then runs Playwright (`apps/web/package.json`: `"e2e": "pnpm run build && playwright test"`). Instead of `webServer`/`go run`/18460, a global setup builds the binary and starts it isolated on a free loopback port, per the orchestrator decision. Evidence: `globalSetup.ts`, `server.ts`, the log above.
- [x] Six specs met: shell, version equals `GET /api/v1/system/info`, Control+K, sidebar toggle with `aria-expanded` and reload persistence, deep link `/does-not-exist` with 200 and not-found view, axe with no serious or critical violations (`e2e/*.spec.ts`, all passed).
- [x] Works from PowerShell on Windows. All runs were in PowerShell, and the environment is passed through `spawn` env (no shell syntax).
- [x] Traces and screenshots on failure go to git-ignored `test-results/` and `playwright-report/` (`trace: "retain-on-failure"`, `screenshot: "only-on-failure"`).
- [x] Gates `lint`, `test` and `e2e` pass.

## Review carry-over from M1-15
- [x] `useSidebarState`: the storage parameter is removed (`useSidebarState(): SidebarState`) and the doc is fixed (diff of `useSidebarState.ts`).
- [x] `systemInfoQuery`: the `staleTime: Infinity` comment is reworded ("keeps showing the previous version until the page is reloaded; that is acceptable").
- [x] Collapsed error indicator: `ServerVersion.tsx` `UnreachableIndicator` renders `AlertIcon` with `text-accent`, `title="Server unreachable"` and an `sr-only` text. Visible: unit test "keeps a visible unreachable indicator with a tooltip while collapsed" (`closest(".sr-only")` is null) and the E2E test 7 (`getByTitle("Server unreachable")` is visible). Accessible: it sits in `role="status"` and has the `sr-only` text. The axe scan of the collapsed state passes.
- [x] `createApiClient` (`packages/api-client/src/client.ts:34-81`, tests in `client.test.ts`):
  - rejects username, username+password, query, empty `?`, fragment and empty `#` (6 `it.each` cases, exact message asserted);
  - never echoes the password: the message is built with `withoutCredentials(url)`, and the test "never repeats the password of a rejected base URL" asserts the message does not contain `hunter2`;
  - also rejects non-http(s) protocols and unparsable URLs.
- [x] `isComposing` guard: `searchShortcut.ts:18`. Test: "Ctrl+K while composing" expects false.
- [x] `event.code === "KeyK"` is used only when `event.key` is not a Latin letter (`searchShortcut.ts:21-24`). Tests: Cyrillic `key:"л" code:KeyK` gives true; Dvorak `key:"k" code:KeyV` gives true; Dvorak `key:"t" code:KeyK` gives false (Ctrl+T is not hijacked); a missing code still works for Latin keys.
- [x] Sidebar footer test is less DOM-coupled: it uses `compareDocumentPosition` (`follows()`) and `closest(".sr-only")` instead of `lastElementChild`.
- [x] webOS `cause` note: comment at `client.ts:48-49`.

## Other checks
- No `TODO`/`FIXME` and no `any` in the new E2E files, `playwright.config.ts` or `client.ts`. The only grep hit was the word "any" in a comment (`server.ts:152`).
- Nothing was staged.

## Failures
None.

## History
- Round 1: PASS.
- Round 2: PASS (see below).

---

# Round 2

Scope: `client.ts` credential and query redaction, the collapsed `ServerVersion` button, sidebar and layout changes, `visibility.ts`, E2E support and specs, task text. No Go code changed; Go gates not run.

## Gates (round 2)

| Gate | Command | Result |
|---|---|---|
| Install | `pnpm -C web install --frozen-lockfile` | "Lockfile is up to date", already up to date |
| Lint | `pnpm -C web lint` | exit 0: eslint clean, dependency-cruiser "no dependency violations found (101 modules, 177 dependencies cruised)", prettier clean, `tsc -b` clean |
| Unit tests | `pnpm -C web test -- --coverage` | 23 files, **295 passed (295)**; coverage 100 % statements (263/263), branches (129/129), functions (74/74), lines (260/260) |
| Build | `pnpm -C web build` | exit 0 |
| E2E run 1 | `pnpm -C web e2e` | **9 passed (5.9 s)** |
| E2E run 2 | `pnpm -C web e2e` | **9 passed (5.9 s)** |

Two further full `pnpm -C web e2e` runs during the cleanup test also gave 9 passed (5.9 s, 5.8 s). The 9 tests are the 8 from round 1 plus "home has no serious or critical axe violations (sidebar collapsed, server unreachable)"; the unreachable-server test is now "keeps an unreachable server visible and reachable in the collapsed sidebar".

No `TODO`, `FIXME`, `any`, `eslint-disable` or `@ts-ignore` in `apps/web/src`, `apps/web/e2e`, `client*.ts` or `playwright.config.ts`. Nothing staged (`git diff --cached --stat` empty).

## Security probe: error messages of `createApiClient`

Throwaway script `scratchpad/probe.mjs` (outside the repo, nothing committed). It runs Node 24 with `--experimental-strip-types`, imports `web/packages/api-client/src/client.ts` and calls `createApiClient({ baseUrl })`. A check flags a leak if a secret substring is both in the input and in the thrown message. Result: no leaks for any input. None of `s3cr3t`, `p%40ss`, `p@ss`, `t0k3n`, `pa/ss`, `abc` appears in any message.

| Input | Thrown message (prefix `Invalid API base URL`, shortened) |
|---|---|
| `ftp://admin:s3cr3t@server.test` | `"ftp://***@server.test": expected an absolute http or https URL ...` |
| `http://admin:s3cr3t@` | `"http://***@": expected ...` |
| `admin:s3cr3t@server.test:8460` | `"***@server.test:8460": expected ...` |
| `http://u:p%40ss@host/path?token=t0k3n#frag` | `"http://host/path": credentials are not allowed.` |
| `https://user:pa/ss#word@host` | `"https://***@host": expected ...` |
| `http://host/?apikey=abc` | `"http://host/": a query is not allowed.` |
| `http://u:p@ss@host/x` | `"http://host/x": credentials are not allowed.` |
| `HTTP://admin:s3cr3t@host` | `"http://host/": credentials are not allowed.` |
| `//admin:s3cr3t@host` | `"***@host": expected ...` |
| `http://host/#abc` | `"http://host/": a fragment is not allowed.` |
| `http://host/path?x=abc@y` | `"http://host/path": a query is not allowed.` |
| `https://admin:s3cr3t@host:99999` | `"https://***@host:99999": expected ...` |
| `  http://admin:s3cr3t@host  ` | `"http://host/": credentials are not allowed.` |
| `javascript://admin:s3cr3t@host/%0aalert(1)` | `"javascript://***@host/%0aalert(1)": expected ...` |
| `http://[::1` | `"http://[::1": expected ...` |
| `http://host\@s3cr3t` | no throw (WHATWG treats `\` as `/`: host `host`, path `/@s3cr3t`, no credentials; correct) |

Observation, not a failure: the `cause` of the unparsable-URL TypeError is the original `URL` error. In Node that object has an `input` property with the raw text. The message is safe, and `client.ts:54-55` documents that `cause` is for diagnostics only. Callers that log `error.cause` verbatim in Node could still print a typed secret; browsers do not expose the input. Low risk, mentioned for the reviewer.

## E2E teardown robustness (abrupt runner kill)

Procedure: start the run, wait until the `mivo` process exists, then `Stop-Process -Force` on the Playwright runner node process (TerminateProcess, so no JS `exit` handler can run). Done twice: once via `pnpm -C web e2e`, once with `node cli.js test` directly (mivo parent = runner pid).

- The server did **not** survive in either case: gone within 500 ms and stayed gone for 5 s. The server log ends mid-traffic with no graceful-shutdown line, so it was killed hard. The cause is not the `process.once("exit")` hook in `server.ts` (it cannot run on TerminateProcess). libuv on Windows puts every non-detached child into a job object with kill-on-close, so the OS ends the server together with its parent. The hook still covers `process.exit` and crashes, and it matters on platforms without that job behaviour.
- The temp directory **does** survive an abrupt kill: `%TEMP%\mivo-e2e-<random>` stays (about 19 MB, mainly the built binary). Teardown cannot run.
- The next run does **not** remove a young stale directory. `removeStaleRoots()` only deletes `mivo-e2e-*` directories older than 6 h (`staleRootAgeMs`), so that it never touches a concurrent run. Evidence: after killing twice, a normal run (9 passed) left both `mivo-e2e-KmIrE8` and `mivo-e2e-SCclpj`. After setting the mtime of `mivo-e2e-KmIrE8` to 7 h ago, the next run (9 passed) removed it and kept the young one. So cleanup works as designed, with a delay of up to 6 h plus the next run. Consequence: each abrupt kill leaks about 19 MB in `%TEMP%` until a run happens after the 6 h mark. This is documented behaviour in the code, not an acceptance criterion; reported as an observation.
- Final state after removing my leftovers by hand: no `mivo` process, no `mivo*` directory in `%TEMP%`, `%LOCALAPPDATA%\Mivo` does not exist, and no `*ivo*` directory under LOCALAPPDATA, APPDATA or ProgramData.

## Round-2 criteria

- [x] All round-1 acceptance criteria still hold (install, lint, test, e2e green; six required specs plus the extras pass; PowerShell works; traces go to the git-ignored `test-results/`).
- [x] Credentials, query and fragment are redacted in every error message of `createApiClient` (probe above, 16 inputs, plus 295 unit tests green at 100 %).
- [x] Collapsed unreachable indicator is a button: tooltip on hover and focus, click or Enter expands the sidebar and focuses the toggle, covered by the E2E test "keeps an unreachable server visible and reachable in the collapsed sidebar", the unit tests and the axe scan of that state (all passed).
- [x] E2E isolation and cleanup verified (see above).

## Failures
None.

REPORT
status: PASS
task: M1-17
files: docs/reports/M1-17-test.md
tests: pnpm -C web install --frozen-lockfile OK; lint OK; test 295/295, coverage 100 % (263/263, 129/129, 74/74, 260/260); build OK; e2e 9/9 on 2 gate runs plus 2 more runs; credential probe with 16 inputs: no leak; abrupt runner kill tested twice
findings:
- No failures. Observation 1: after an abrupt kill (TerminateProcess) the mivo server dies with the runner on Windows (libuv kill-on-close job object), but its mivo-e2e-* temp dir (about 19 MB) stays, and the next run only removes such directories older than 6 h (verified by backdating one). Intended by the code; leak is bounded.
- Observation 2: the TypeError `cause` of an unparsable base URL holds the raw input in Node (`cause.input`); the message is safe. Callers must not log `cause` blindly.
- Cosmetic: dependency-cruiser global-install warning and Node NO_COLOR/FORCE_COLOR warnings.
next: proceed to mivo-reviewer
