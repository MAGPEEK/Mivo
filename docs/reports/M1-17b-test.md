# M1-17b test report

Task: `docs/tasks/M1-17b-client-url-redaction-and-indicator-a11y.md`
Round: 2
Verdict: **PASS** (all gates green, all acceptance criteria met, no leak in 59 probe inputs)

## Gates

| Gate | Command | Result |
|---|---|---|
| Lint (ESLint, dependency-cruiser, Prettier check, tsc -b) | `pnpm -C web lint` | PASS, exit 0; "no dependency violations found (103 modules, 179 dependencies cruised)"; "All matched files use Prettier code style!" |
| Unit tests + coverage | `pnpm -C web test -- --coverage` | PASS; 24 files, 335 tests; Statements 100 % (288/288), Branches 100 % (138/138), Functions 100 % (85/85), Lines 100 % (285/285); "No files with missing coverage" |
| Build | `pnpm -C web build` | PASS, exit 0 ("built in 318ms") |
| E2E, 6 consecutive runs | `pnpm -C web e2e` | PASS, 10 passed in every run (6.8 s, 6.3 s, 6.6 s, 6.5 s, 6.3 s, 7.7 s), exit 0 each |
| Go / Docker / media | not applicable (web-only task) | skipped |

The formatting check is part of `pnpm -C web lint` (`lint:format`, `prettier --check .`).

### E2E flakiness check

The implementer saw one unidentified failure ("1 failed, 9 passed") in 11 runs. After each run the `playwright-report` and `test-results` directories were copied into the scratchpad (`...\scratchpad\r2-e2e-run1` to `r2-e2e-run6`, each with `output.txt` and `exit.txt`). The task asked for at least two runs; six were done. All six were green, so the failure could not be reproduced and cannot be identified. Judgement: not a finding that blocks PASS. The suite builds the real binary and starts a real server for each run, and the implementer's failure happened while a parallel agent was loading the machine. Possible sources are the port picked by `findFreePort` being taken between probe and server start (this would fail loudly as "exited before healthy"), or a timing-sensitive pointer or hover test under load. Neither can be confirmed. Recommendation: if it shows up again, keep the Playwright report (`trace: on-first-retry` or similar) so the failing test is named. Not a defect of this task.

## Independent redaction probe

Script `C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\probe-r2.mjs` (not in the repo, not committed). It imports the real `client.ts` through Node type stripping (Node v24.22.1). Command: `node probe-r2.mjs`; result: "inputs: 59 (round1: 29 leak/new: 30) failures: 0", exit 0.

Inputs:
- The 29 round-1 inputs (task inputs, full-width `＠`, `admin:secret@host`, `javascript:`, `data:`, 10 000-character variants and others).
- The reviewer's leak cases, each with `@`, full-width `＠` and `%40`: `http://admin:4711#pw@server.test`, `http://admin:4711?pw@server.test`, `http://admin:/pw@server.test?x`, plus an https variant with a path.
- `http://server.test/api;token=SECRET?x`, the same with `#x`, `http://admin:4711@server.test/api;token=SECRET`, upper-case scheme, a leading space, `http://admin:4711/#pw@server.test`, `http://admin:4711/?pw@server.test#x`, `http://admin%3A4711#pw@server.test`, `ftp://admin:4711#pw@server.test`, `ftp://admin:4711?pw@server.test`, `admin:4711#pw@server.test`, `admin:4711://pw@server.test`.
- Scheme lengths 1, 19, 20, 21, 22 and 40 (`abcdefghijklmnopqrs://server.test/?x` and so on).

Assertions per input, on message, stack header (frame lines with file paths removed), every own property (including symbols), and `cause`:
- none of `admin`, `4711`, `pw`, `secret`, `token` (case-insensitive) appears, and neither does the raw input or its first 40 characters. The one allowed exception is a shown scheme that the user typed as `scheme://` and that itself contains the word; this did not occur in any input;
- the error is a `TypeError`, has no `cause`, own keys are exactly `stack,message`;
- the message is at most 250 characters;
- the message matches exactly one of the documented forms (regular expression of the four phrases plus optional `got the scheme "..."` with 1 to 20 characters).

Observed behaviour:
- `http://admin:4711#pw@server.test` gives `a fragment is not allowed.`; the `?` variant gives `a query is not allowed.`; `http://admin:/pw@server.test?x` gives `a query is not allowed.` The `＠` and `%40` variants behave the same. No origin or path appears.
- `http://server.test/api;token=SECRET?x` gives `a query is not allowed.`
- Scheme shown for 1, 19 and 20 characters, not shown for 21, 22 and 40 characters. The colon is not counted (20 characters shown, 21 hidden), so the round-1 off-by-one is fixed.
- Non-http(s) input that does not start with `scheme://` (for example `media.local:8460/...`, `admin:secret@host`, `javascript:...`, `data:...`, `admin:4711://pw@server.test`) gives the message without a scheme.
- Accepted: `""`, `http://h`, `https://h:8460/api`, `http:server.test`.

## `@throws` doc against observed behaviour

The block at `web/packages/api-client/src/client.ts:39-55` was checked point by point:
- Exceptions are `TypeError` only, with one of the four named problems: confirmed by the message regular expression above.
- "repeats nothing of the input except the scheme, only when the input starts with `scheme://` and the scheme has at most 20 characters": confirmed (19/20 shown, 21 not; `media.local:8460`, `localhost:8460`, `javascript:`, `data:`, `admin:secret@host` show nothing).
- "Nothing is shown when the input does not parse; the scheme is not followed by `//`; the scheme is longer than 20 characters; the input is an http(s) URL with credentials, a query or a fragment": confirmed for all four.
- "The error has no `cause`": confirmed (own keys `stack,message`).
- Grammar of the sentence from round 1 is fixed. No imprecision found. One harmless detail: an upper-case scheme in the input is shown in lower case (the parser normalises it); the doc says "that scheme", which is still accurate.

## Acceptance criteria

1. **Base URLs never echoed raw, message capped. MET.** `client.ts` has `maxShownSchemeLength = 20`; the messages are built from constants only, so no host, port, path, query or fragment is repeated. `client.test.ts` covers the task inputs, full-width `＠`, 10 000-character input and the scheme boundary 19/20/21 (lines 123-125). The probe confirms it.
2. **No raw input in `cause`. MET.** `catch {` drops the parser error. `client.test.ts:196` asserts `error.cause` is undefined; `client.test.ts:200` asserts no `admin|4711|pw|SECRET|token` in any own property. The probe confirms own keys `stack,message`.
3. **`@throws` doc matches behaviour exactly. MET.** See the section above.
4. **`aria-expanded={false}` and `aria-controls` through `Sidebar`. MET (unchanged since round 1).** Covered by `AppLayout.test.tsx`, `Sidebar.test.tsx` and the e2e axe test "sidebar collapsed, server unreachable". All pass.
5. **Tooltip meets WCAG 1.4.13. MET.** Dismissible with Escape without moving focus, via a capture-phase document listener that is attached only while open (`useDismissibleTooltip.ts:37-50`). New unit test "closes on Escape even when a handler stops it from bubbling" (`useDismissibleTooltip.test.ts:88`) and "listens for Escape only while open" (`:128`). Hoverable and persistent: e2e test "keeps the unreachable tooltip open while the pointer moves onto it" passed in 6 of 6 runs.
6. **Nits. MET.** `AppLayout.test.tsx` asserts `queryByRole("button")` is null. `e2e/support/server.ts`: `buildError` distinguishes `ERR_CHILD_PROCESS_STDIO_MAXBUFFER` from timeout (`killed`) (lines 196-214); the log handle is closed when `spawn` throws synchronously (lines 297-301); the comment above `stopProcess` (lines 323-325) says that SIGTERM is a hard kill on Windows.
7. **Gates. MET.** See the table above (e2e run six times instead of twice).

## Scope notes

- `web/apps/web/e2e/shell.spec.ts` is outside the task's "Files/areas" list. Acceptable, as in round 1: the tooltip criteria need browser verification. The task's file list should name `e2e/**` (docs nit).
- Round 1 observation (path echo for rejected http(s) URLs) is resolved by the orchestrator decision: origin and path are no longer echoed at all.
- Accepted limitation, by decision: a scheme the user typed as `scheme://` (at most 20 characters) is shown. If a user types a secret as the scheme of a `secret://...` URL, it is repeated; this is the documented exception.

## History

- Round 1: gates green, 1 minor doc finding (`@throws` grammar and gaps), verdict FAIL. Reviewer then found a credential leak (userinfo read as host:port, echoed through origin/path), a scheme-length off-by-one, a bubble-phase Escape listener and a misleading comment in `server.ts`.
- Round 2: all of the above fixed; lint, tests (335, 100 %), build and 6 e2e runs green; probe with 59 inputs clean; verdict PASS.

REPORT
status: PASS
task: M1-17b
files: docs/reports/M1-17b-test.md
tests: pnpm -C web lint exit 0 (prettier check included); pnpm -C web test -- --coverage 335/335 passed, 100 % statements/branches/functions/lines; pnpm -C web build exit 0; pnpm -C web e2e x6, 10/10 passed each; independent redaction probe 59 inputs, 0 failures
findings:
- None blocking. All round-1 findings and the reviewer's leak cases are fixed: no admin/4711/pw/secret/token/raw input in message, stack, cause or own properties; scheme shown at 19/20 characters, hidden at 21; error has no cause; @throws doc matches observed behaviour exactly.
- E2E flake from the implementer ("1 failed, 9 passed") not reproducible in 6 consecutive runs (reports archived in the scratchpad, r2-e2e-run1..6); judged a load-related one-off, not a finding against this task. Keep the Playwright report if it recurs.
- Accepted by decision: a user-typed `scheme://` scheme of at most 20 characters is echoed.
- e2e/shell.spec.ts is outside the task's file list; acceptable, docs nit only.
next: Hand over to mivo-reviewer for the round-2 review; no implementer changes needed.
