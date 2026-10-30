# M2-26 test report: REST API libraries, scans and items

Verdict: **PASS** (round 1, tested together with M2-28)

Isolation: clean `git worktree` at HEAD 5e64b56 plus exactly the 20 files of
`m2-26-28-files.txt` (restapi library/items/playback/problem files, `api/openapi.yaml`,
`apigen.gen.go`, `web/packages/api-client/src/generated/schema.ts`). The copies were byte-identical
to the main tree (hash check). The worktree is removed.

## Gates

| Gate | Result |
|---|---|
| `go build ./...`, `go vet ./...` | clean |
| `go test "-coverprofile=coverage.out" ./...` (Windows, no cgo) | all packages ok; `restapi` 99.1 % |
| `go-test-coverage --config=.testcoverage.yml` | PASS, total 97.1 % (threshold 70 %), package threshold PASS |
| `golangci-lint run --allow-parallel-runners` (incl. depguard) | 0 issues |
| `golangci-lint fmt --diff` | clean |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | clean |
| archtest (`go test ./internal/archtest/...`) | ok |
| Linux race (`docker run golang:1.27.3 go test -race ./...`) | all packages ok, incl. `restapi`, `app/*` |
| `go generate ./...` + `pnpm -C web gen`, then `git diff --exit-code` | clean (no drift in `apigen.gen.go`, `schema.ts`) |
| `pnpm -C web lint:deps`, `lint:format` | clean |
| `pnpm -C web lint` (eslint, tsc) / `pnpm -C web test` | see note below: failures only in `ItemPage.test.tsx` / `router.test.tsx`, identical at clean HEAD |
| `pnpm -C web e2e` | 10 passed |

Note on web lint/test (not caused by M2-26/M2-28): `apps/web/src/features/item/components/ItemPage.test.tsx`
(lines 5, 324-342) imports `createFakeServer` and `jsonResponse` from `src/test/fakeApi.ts`. At HEAD
`fakeApi.ts` does not export them (the helpers exist only as an uncommitted change in the main tree,
wave M2-22). Effects: eslint 11 errors, `tsc -b` TS2305 x2 + TS7031, vitest 2 failed of 775
(`router.test.tsx > renders /items/item-1 inside the app layout`, `ItemPage.test.tsx > receives the
item ID from the route`). I reproduced the same 2 vitest failures with the M2-26/28 changes stashed
(clean HEAD), so the api-client change (schema.ts) is not involved. The orchestrator must commit the
`fakeApi.ts` change together with the M2-22 work, otherwise HEAD is red for web lint/test.
All other 773 web tests pass.

## Acceptance criteria

1. **Ports satisfied, stubs replaced - met.** `library_fakes_test.go` has compile-time assertions
   `var _ restapi.LibraryService = (*applibrary.Service)(nil)` and
   `var _ restapi.ScanService = (*appscan.Service)(nil)`; build is green. `library.go`/`items.go` call
   `api.library`/`api.scan`, no `notImplemented` once wired; nil services still answer 501 (tested).
2. **List/get libraries combine scan status; unknown ID 404 - met.** `TestListLibraries`,
   `TestGetLibrary`, `TestGetLibraryUnknownIDAnswersNotFound` green. My probe: GetLibrary with
   `ErrNotFound` gives 404 `not_found`. The root path never appears in the response (test with
   `testRoot`).
3. **Scan trigger 202, 503 during shutdown - met.** Probe: `Start` returning
   `domain.ErrUnavailable` gives 503 `unavailable`; `ErrNotFound` 404; `ErrInvalid` 500; success 202
   with status. `app/scan` `ensureOpen` wraps `ErrUnavailable` after `Close` (service.go:208).
4. **Items paging and item detail - met.** Probes (fake service, call recorded): `limit=0`, `501`,
   `-1`, `abc`, `2147483648` and a cursor of 2049 characters all give 400 and the service was called
   0 times; `limit=1`, `limit=500` and a cursor of exactly 2048 characters give 200. A cursor of
   about 1470 characters is passed to the service unchanged and returned unchanged as `nextCursor`.
   `nextCursor` key absent when the page has no more items (probe body has no `nextCursor`). Service
   errors: `ErrNotFound` 404, `ErrInvalid` 400 `invalid`, other 500. `GET /items/{id}`:
   `TestGetItem`, `TestGetItemSendsFileNamesOnly` (names with `/` and `\` reduced to the last
   element, asserted to contain neither), `TestGetItemErrors`.
5. **Durations in seconds with ms precision; timestamps RFC 3339 UTC - met.** Response
   `durationSeconds` 5423.042 for 1:30:23.042123456; probe: scan `startedAt` of
   `2026-01-02T03:04:05.6789+02:00` is sent as `"2026-01-02T01:04:05.678Z"`; `finishedAt` is omitted
   while a scan runs.
6. **Handler tests with fake services, all status codes, JSON shape, truncated paths - met.**
   `library_test.go`, `items_test.go`, `items_response_internal_test.go` use `decodeStrictly` against
   the generated types and cover 200/202/400/404/500/501/503. `TestLibraryLogsCutLongClientText`
   green. My probe with a 5000-character path and cursor: the log lines carry the path cut to about
   200 characters with an ellipsis and the parser error text cut in the middle; no line exceeded 1500
   characters (M2-08).
7. **Gates - met** (see table).

Orchestrator notes of the task:
- Stored-data errors are 500, not 400 - met: probe with unknown library kind in `ListLibraries` and
  `GetLibrary` gives 500 `internal_error`; `GetLibrary`/`ListLibraries`/`GetItem` returning
  `domain.ErrInvalid` also give 500; only `ErrNotFound` yields 404 (`operationFailure`,
  `library_failure.go`). `TestLibraryReadFailuresAreInternal`, `TestCorruptStoredLibrariesAreInternal`
  cover it.
- openapi descriptions (ScanStatus `finishedAt`, `errorCode` list, `failed` count) - met
  (`git diff` of `api/openapi.yaml`; version 1.1.0 to 1.1.1).
- Cursor `maxLength` 2048 for the query parameter and `nextCursor`, server and client regenerated -
  met (no codegen drift); round trip above 512 characters tested (`TestListLibraryItemsCursorRoundTrip`
  and my probe).
- Handlers use the `app/library` service, never a repository - met (`LibraryService` port; no
  sqlite import in `restapi`).
- `playback.go` and `api.go` not edited by this task - met (api.go unchanged against HEAD).

## Findings

None for M2-26. (Out-of-scope observation: the web lint/test red state at HEAD, see note above.)

## History

- Round 1: PASS.
