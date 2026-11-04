# M3-19n test report (round 1): PASS

- pnpm -C web lint: exit 0 (eslint, depcruise, prettier, tsc)
- pnpm -C web test: 96 files, 1405 tests passed; coverage 99.23 % stmts / 97.35 % branches (thresholds met)
- pnpm -C web build: OK
- pnpm -C web gen: no generated diff (only hand-written api-client files modified: apiError.ts, index.ts, schemas.ts, requestData.test.ts)
- go vet -tags webui ./...: clean
- e2e (--no-deps --project=chromium): 1 failed (library.spec.ts:32, known pre-M3-18 breakage), 30 did not run (serial abort after it). Not attributable to M3-19n.
- Acceptance criteria: dialog, folder browser, permission help, save/problem mapping and tests covered by LibraryDialog.test.tsx, libraryDraft.test.ts, useModalFocus.test.tsx (all green, incl. axe); gates met. Not manually exercised in a browser.

## History
Round 1: PASS
