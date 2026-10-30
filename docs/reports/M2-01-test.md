# M2-01 Test Report - API contract for libraries, items and playback sessions

Status: **PASS** (round 2)
Date: 2026-10-30
Toolchain: go1.27.3 windows/amd64 (cgo on, race works), golangci-lint 2.15.0, Node 24 / pnpm 10, Playwright chromium shell
Scope: `api/openapi.yaml` (1.1.0), `api/spec_test.go`, `internal/adapters/http/apigen/**`,
`internal/adapters/http/restapi/**`, `internal/domain/errors*.go`, `cmd/mivo/serve.go` (one call),
`internal/adapters/http/server/server_test.go` (one line), `web/packages/api-client/src/**`, `go.mod`/`go.sum`,
`docs/adr/0024` (Notes only).

## Scope note for the repo-wide Go gates (parallel work)

M2-10 (`internal/adapters/sqlite`) and M2-11 (`internal/adapters/ffmpeg`) are being implemented in parallel and are
not part of this verdict. Observed state during this run, reported separately:

| Package | State | Evidence |
|---|---|---|
| `internal/adapters/sqlite` | test fails | `go test -race ./...`: `migrate_test.go:31: schema version = 2, want 1`; `:35 applied migration rows = 2, want 1`; `:44 log records ... want 1`. The existing M1 test still expects one migration while `00002_media_catalog.sql` (M2-10) now exists. Its non-test code builds; the real binary applies migration 2 fine. |
| `internal/adapters/ffmpeg` | does not compile (tests) | `go vet -tags webui ./...`: `faketool_internal_test.go:162:42: undefined: tool` (and `maxOutputBytes`, `describe`, `frameRate`, `parseSeconds`); the package has only test files and `testdata` right now. |

Repo-wide gates therefore ran twice where it mattered: once over `./...` (failures listed above, all in the two
parallel packages) and once over `./...` minus `internal/adapters/sqlite` and `internal/adapters/ffmpeg`
(`go list ./... | grep -v 'adapters/(sqlite|ffmpeg)'`). Everything else in the repository is green. Earlier in this
same run `go build ./...` and `go vet ./...` (untagged) were still clean over everything; the ffmpeg test files
changed under the run afterwards.

## Gates

| Gate | Command | Result |
|---|---|---|
| build | `go build ./...` | exit 0 |
| vet | `go vet <all packages except sqlite/ffmpeg>` (and `-tags webui`) | exit 0 both. Unscoped `go vet ./...` was exit 0 at the start of the run; unscoped `-tags webui` now fails only in the ffmpeg/sqlite test files (above) |
| test + race + coverage | `go test -race -count=1 "-coverprofile=coverage.out" <scoped>` | exit 0, all `ok`; restapi 98.2 %, server 97.2 %, cmd/mivo 88.9 %, domain/{library,media,system} 100 % |
| test over `./...` | same, unscoped | exit 1, only `internal/adapters/sqlite` FAIL (not M2-01, see scope note); all other packages `ok` |
| coverage gate | `go tool "-modfile=tools/go.mod" go-test-coverage "--config=.testcoverage.yml"` (profile of the scoped run) | `Total coverage threshold (70%) satisfied: PASS`, `Total test coverage: 97.9% (2234/2283)`; per-package domain override 90 %: all domain packages 100 % |
| lint | `golangci-lint run` | `0 issues.` at the start of the run (before ffmpeg tests broke). After that the unscoped run only prints typechecking errors for `internal/adapters/ffmpeg`; scoped re-run over `./api/... ./cmd/... ./internal/adapters/http/... ./internal/domain/... ./internal/platform/... ./internal/adapters/fs/... ./internal/archtest/...`: `0 issues.` |
| tagged lint | `golangci-lint run --build-tags webui ./cmd/... ./internal/adapters/http/...` | `0 issues.` |
| format | `golangci-lint fmt --diff` | exit 0, no diff |
| tidy (root) | `go mod tidy -diff` | exit 0, no diff |
| tidy (tools) | `go -C tools mod tidy -diff` | exit 0, no diff |
| vulnerabilities | `go tool "-modfile=tools/go.mod" govulncheck ./...` | `No vulnerabilities found.` |
| generation drift | SHA-256 of all 8 files in `internal/adapters/http/apigen`, `web/packages/api-client/src/generated`, `internal/adapters/sqlite/sqlcgen` before and after `go generate ./...` and `pnpm -C web gen` | both exit 0, 0 differences |
| api spec tests | `go test -count=1 -v ./api/` | all 20 tests PASS, including `TestNoValueIsTakenFromStreamURL` |
| mutation check | see below | all 6 mutations caught by `TestNoValueIsTakenFromStreamURL` |
| web lint | `pnpm -C web lint` | eslint clean, dependency-cruiser `no dependency violations found (124 modules, 219 dependencies cruised)`, prettier clean, `tsc -b` clean |
| web test | `pnpm -C web test` | 27 files, 389 tests passed; Statements 100 % (332/332), Branches 100 % (153/153), Functions 100 % (106/106), Lines 100 % (329/329) |
| web build | `pnpm -C web build` | exit 0 |
| e2e (run 1) | `pnpm -C web e2e` | 10 passed (6.3 s) |
| e2e (run 2) | `pnpm -C web e2e` | 10 passed (6.5 s) |
| tagged tests | `go test -race -count=1 -tags webui ./internal/adapters/http/... ./cmd/...` | all `ok` |
| layer rule | `go list -deps ./internal/domain/...` filtered for `database/sql\|net/http\|os/exec\|adapters\|internal/app` | no match |
| TODO scan | `TODO\|FIXME` in api, restapi, domain, cmd, api-client src | 0 hits |
| media corpus / docker | skipped | task touches neither playback nor packaging |

The generated files are still uncommitted, so `git diff --exit-code` against HEAD cannot be clean for them; the
hash comparison (regenerate, compare) shows the working tree is exactly what the generators produce.

## Mutation check of TestNoValueIsTakenFromStreamURL

Method: a copy of `api/`, `internal/domain`, `internal/platform/brand`, the apigen `oapi-codegen.yaml`, `go.mod`
and `go.sum` in the scratchpad (`...\scratchpad\mut`), so no repository file was touched. Baseline in the copy:
`ok`. Each mutation edits `openapi.yaml` of the copy only; the target test must fail.

| # | Mutation | Test result |
|---|---|---|
| M1 | `SessionId` description: "ID of the playback session, as found in the streamUrl." (the round-1 wording) | FAIL: `parameter SessionId ... mentions streamUrl` (also once per operation using it) |
| M2 | `SessionId` description: "..., taken from the streamUrl." | FAIL: `mentions streamUrl` |
| M3 | `ItemId` description: "ID of the item, see streamUrl." | FAIL: `parameter ItemId mentions streamUrl` |
| M4 | `stream` tag description: "Take the session ID out of the streamUrl." | FAIL: `tag stream presents streamUrl as a source of values` |
| M5 | `playback` tag description: "read values from its streamUrl." | FAIL: `tag playback presents streamUrl as a source of values` |
| M6 | `PlaybackSession.durationSeconds` property description: "also in the streamUrl." | FAIL: `PlaybackSession.durationSeconds mentions streamUrl` |

Mutated copy restored afterwards; baseline passes again. Limit of the test (not a failure): the check is on the
literal token `streamUrl`. Wording that says "stream URL" (with a space) as a source, for example "taken from the
stream URL" in a parameter description, passes. The real spec has no such text.

## Round-2 changes verified

| Change | Evidence |
|---|---|
| `SessionId` no longer points at streamUrl; new `TestNoValueIsTakenFromStreamURL` | `openapi.yaml:413-420`: "ID of the playback session (PlaybackSession.id)."; test passes; mutations above |
| `streamUrl` and `Location` are URI references resolved per RFC 3986 section 5 | `openapi.yaml:1301-1310` (`format: uri-reference`, description); ADR 0024 "Notes (M2-01)" says the same and names the reverse-proxy prefix follow-up before 7b |
| 503 text: Retry-After only promised for `session_limit_reached` | `openapi.yaml:246-253` ("Retry-After accompanies session_limit_reached"), `:219-221` |
| `oapi-codegen.yaml` comments | diff read: stream routes comment, uuid mapping comment points to `TestUUIDPathParameterBinding` |
| Chunked non-JSON body > 64 KiB is 413 | `api.go:105-118` (`isBodyTooLarge(err) \|\| remainingBodyTooLarge(request)`), `bodylimit.go:46-58`; live probes below; `TestRequestBodyLimitOnOperations/{unannounced_65_KiB_of_non-JSON,unannounced_64_KiB_of_non-JSON,unannounced_small_non-JSON,announced_65_KiB_of_non-JSON}` |
| `nextCursor` and `cursor` `maxLength: 512` | `openapi.yaml:179,764` |
| `startPositionSeconds` documented | `openapi.yaml:1312-1320` |
| Debug log for mapped 4xx | `api.go:133-150` (`default:` branch logs "api request rejected" at debug; 500 error, >500 warn); not visible at the default INFO level in the live log, as intended |
| Mount patterns outside `/api/` rejected | `options.go:63-65`, `patternPath`; `TestNewHandlerRejectsInvalidOptions/{mount_outside_the_API,mount_without_method_outside_the_API,mount_on_the_API_prefix_without_its_slash,mount_with_a_host,...}`, `TestMountPatternForms` |
| `ApiClient` doc note | `client.ts:1-13`; no behaviour change |

## Black-box run with the real binary

Binary built from the working tree into `C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\bb2\mivo.exe`,
isolated `MIVO_CONFIG_DIR`/`MIVO_PATHS_*` in the same folder, `MIVO_SERVER_HOST=127.0.0.1`, port 18463. Stopped afterwards.
The server log has one `http request` line per call; zero `ERROR`/`WARN`/`panic` lines and no request text.

Round-1 probes (all unchanged):

| Request | Result |
|---|---|
| `GET /api/v1/health`, `/system/info` | 200, health `{"checks":[{"name":"database","status":"ok"}],"status":"ok"}`; `apiVersion":"1"` |
| libraries list/get, `POST .../scan`, items (also `?limit=50&cursor=abc`), item, `POST /playback/sessions` (valid body), session GET/DELETE | 501 `application/problem+json` `{"code":"not_implemented","status":501,"title":"Not Implemented","type":"about:blank"}` |
| stream `GET`/`HEAD .../file`, `GET .../hls/index.m3u8` | 404 `not_found` problem (no mount wired until M2-29/M2-32) |
| `not-a-uuid` in libraryId (GET, items, scan), itemId, sessionId (GET, DELETE); 35-char truncated UUID; `%00%ff` | 400 `{"code":"bad_request","status":400,"title":"Bad Request","type":"about:blank"}`, no detail |
| `?limit=abc` | 400 `bad_request` |
| `{bad` JSON, empty body | 400 `bad_request` |
| `PUT /api/v1/libraries` | 405 `method_not_allowed` problem+json |
| `GET /api/v1/nope` | 404 `not_found` problem+json |

Body limit on `POST /api/v1/playback/sessions` (66,560 bytes = 65 KiB, 65,536 = 64 KiB):

| Body | Round 1 | Round 2 |
|---|---|---|
| 65 KiB JSON-shaped, known length | 413 | 413 `payload_too_large` |
| 65 KiB JSON-shaped, chunked | 413 | 413 |
| 65 KiB non-JSON, known length | 413 | 413 |
| **65 KiB non-JSON, chunked** | 400 | **413 `payload_too_large`** |
| exactly 64 KiB non-JSON, known length | 400 | 400 `bad_request` (limit inclusive) |
| exactly 64 KiB non-JSON, chunked | not probed | 400 `bad_request` |
| 64 KiB + 1 non-JSON, known length | 413 | 413 |
| 64 KiB + 1 non-JSON, chunked | not probed | 413 |
| small non-JSON (`hello`), known length | not probed | 400 `bad_request` |
| small non-JSON, chunked | not probed | 400 `bad_request` |
| `POST .../scan`, 65 KiB known length | 413 | 413 |
| `GET /health`, 65 KiB known length | 413 | 413 |

Unchanged observation (not a regression, not blocking): a chunked oversize body on handlers that never read a body
(`POST .../scan`, `GET /health`) is served normally (501 / 200); a known-length oversize body gets 413 everywhere.

## Acceptance criteria

| # | Criterion | Result | Evidence |
|---|---|---|---|
| 1 | Spec 3.0.3, version `1.1.0`, all listed operations with operationIds, descriptions, examples; `limit` 1-500 default 100, opaque `cursor`; 201/202; stream routes; `fileName` pattern | Met | `TestSpecOperations`, `TestPagingParameters`, `TestHLSFileNamePattern`, `TestSpecIsValidOpenAPI` (validates examples), `TestSpecHeader` |
| 2 | Schemas `Library` ... `PlaybackReason`; UUID IDs; extensible enums | Met | `TestMediaFileExposesNoPath`, `TestClientProfileIsBounded`, `TestMediaIdentifierPattern`, `TestIDsAreUUIDs`, `TestEnumsAreExtensible`, `TestEverySchemaIsDescribed` |
| 3 | `streamUrl` documented as opaque; stream routes `security: []` | Met | `openapi.yaml:1301-1306`; `TestStreamURLIsOpaque`, `TestNoValueIsTakenFromStreamURL`, `TestStreamOperationsAreExcludedFromTheStrictServer` |
| 4 | New problem codes documented and as `restapi` constants; `ErrUnsupported` 422, `ErrUnavailable` 503 in the mapping test | Met | `TestPublishedProblemCodesAreDocumented`, `TestProblemForError`, `domain/errors_test.go` (all green under `-race`) |
| 5 | `oapi-codegen.yaml` excludes the stream operations; no generation drift | Met | `oapi-codegen.yaml:22-25`; hash comparison 0 differences |
| 6 | `NewHandler` takes an options struct with `Mounts`; nil services -> 501 `not_implemented`; stubs and placeholder interfaces | Met | `options.go`, `api.go`, live 501 answers; `TestLibraryOperationsAnswerNotImplemented`, `TestPlaybackOperationsAnswerNotImplemented`, `TestNewHandlerRejectsInvalidOptions` (11 cases) |
| 7 | Every API body limited to 64 KiB, larger -> 413 `payload_too_large` (65 KiB test) | Met (round-1 caveat resolved) | `bodylimit.go`, `api.go:105-118`; `TestRequestBodyLimitOnOperations` (8 cases), `TestRequestBodyLimitOnMounts`; live table above: 413 for known-length, chunked JSON and chunked non-JSON 65 KiB; exactly 64 KiB still 400 |
| 8 | HTTP test binds a `uuid` path parameter; `not-a-uuid` -> 400 problem without internal text | Met | `TestUUIDPathParameterBinding`, `TestMalformedParametersAnswerBadRequest` (6 cases), `TestMalformedRequestHandlersWriteProblemsWithoutErrorText`; live 400 with fixed body |
| 9 | Mounted test handler reachable; wrong method -> 405 problem | Met | `TestMountsAreServedWithTheAPIRouting/{matching_request,wrong_method,path_below_the_mount,contract_routes_still_work}` |
| 10 | api-client exports the new types; BACKLOG "api-client nits" done | Met | `index.ts`, `schemas.test.ts`, `client.test.ts` (389 web tests, 100 % coverage) |
| 11 | `spec_test.go` asserts every `/api/v1/` path has an operationId and every non-stream operation a `default` problem response | Met | `TestEveryPathCarriesAPIVersion`, `TestEveryOperationIsNamedAndDescribed`, `TestEveryOperationDeclaresProblemDefault` |
| 12 | Tests: restapi table tests, api-client Vitest | Met | test names above |
| 13 | Gates: Go lint, race tests, coverage gate, web lint/test, drift check | Met | gate table (Go gates over the repository without the two parallel packages) |

BACKLOG note (unchanged): `docs/tasks/BACKLOG.md` still lists "api-client nits -> M2-01" in the schedule line; the
orchestrator removes it on commit.

## Findings

No blocking findings in the M2-01 scope. Non-blocking:

1. `TestNoValueIsTakenFromStreamURL` matches the literal token `streamUrl` only; "stream URL" as a value source in a
   description would pass (see mutation check). Optional hardening.
2. After a decoder error `remainingBodyTooLarge` reads up to 64 KiB more from the request body. This is bounded by
   the body limit; the server has `ReadHeaderTimeout` but no `ReadTimeout` (documented decision for streaming), so a
   client that trickles bytes can hold such a request open, exactly as it can for the JSON decoder today. No change
   needed now; relevant for M2-08 if a body read deadline is introduced.
3. Chunked oversize bodies on handlers that never read a body are served normally (harmless today).
4. `limit=0` / `limit=501` is not rejected by the generated binder; range enforcement (400 `invalid`) belongs to M2-26.
5. `restapi.WriteProblem` has an unreachable error branch; package coverage is unaffected.
6. The `416` response of `getPlaybackFile` has no body schema (only `default` carries problem+json); note for M2-29.
7. Parallel work breaks repo-wide gates right now: `internal/adapters/sqlite` `TestMigrateCreatesTheSchemaAndASecondRunIsANoOp`
   still expects schema version 1 (`migrate_test.go:31,35,44`) and `internal/adapters/ffmpeg` test files reference
   symbols that do not exist yet (`faketool_internal_test.go:73,162`, `probe_internal_test.go:47`). Owners M2-10 / M2-11
   must fix these before their own gates.

## Housekeeping

`coverage.out` (git-ignored, this run's artefact) was deleted at the end. The git-ignored `mivo.exe` in the repo
root is not from this run (the binary of this run is in the scratchpad). `coverage-scratch` from round 1 does not
exist. No production code or test file was edited; nothing was staged or committed.

## History

- Round 1 (2026-10-30): PASS. 13 criteria met, one caveat: a chunked non-JSON body above 64 KiB answered 400 instead
  of 413 (finding 1). Gates: all green over the whole repository, e2e 10 passed twice, black-box run matched.
- Round 2 (2026-10-30): PASS. Round-2 changes verified; chunked non-JSON 65 KiB now 413 live; the new spec test
  catches all 6 mutations; repo-wide Go gates run without the parallel M2-10 / M2-11 packages, which currently fail.

REPORT
status: PASS
task: M2-01
files: docs/reports/M2-01-test.md
tests: go build ./... ok; go vet (scoped, and -tags webui scoped) ok; go test -race -count=1 -coverprofile on ./... minus adapters/sqlite and adapters/ffmpeg all ok (unscoped: only adapters/sqlite fails, M2-10); go-test-coverage PASS (total 97.9 %, 70 % required; domain packages 100 %); golangci-lint run 0 issues (full at start, scoped after ffmpeg tests broke, also -tags webui); fmt --diff, go mod tidy -diff (root + tools) clean; govulncheck none; go generate + pnpm -C web gen 0 drift (hash comparison); api tests 20/20 green; TestNoValueIsTakenFromStreamURL catches 6/6 mutations; pnpm -C web lint clean, test 389 passed at 100 %, build ok, e2e 10 passed twice; real binary: chunked non-JSON 65 KiB 413, small non-JSON 400, exactly 64 KiB 400, all round-1 probes unchanged
findings:
- All 13 criteria met; the round-1 caveat (chunked non-JSON > 64 KiB answered 400) is fixed: 413 live and in TestRequestBodyLimitOnOperations; exactly 64 KiB stays 400.
- All round-2 changes verified in spec, code, ADR 0024 notes and tests (SessionId wording, URI-reference docs, 503/Retry-After text, maxLength 512, startPositionSeconds, debug log, mount prefix check, ApiClient doc).
- Non-blocking: TestNoValueIsTakenFromStreamURL only matches the literal token "streamUrl" ("stream URL" would pass); remainingBodyTooLarge reads up to 64 KiB after a decoder error with no ReadTimeout (bounded, same exposure as the JSON decoder).
- Out of scope but failing now: internal/adapters/sqlite TestMigrateCreatesTheSchemaAndASecondRunIsANoOp (migrate_test.go:31,35,44 expects version 1, migration 2 exists; M2-10) and internal/adapters/ffmpeg tests do not compile (faketool_internal_test.go:73,162, probe_internal_test.go:47; M2-11). Unscoped go vet -tags webui ./... and golangci-lint run fail only because of these.
- BACKLOG.md line 9 still names "api-client nits -> M2-01"; remove on commit.
next: Send to mivo-reviewer. M2-10 and M2-11 owners must fix the sqlite test and the ffmpeg compile errors before their own gates; rerun unscoped Go gates once both packages are green.
