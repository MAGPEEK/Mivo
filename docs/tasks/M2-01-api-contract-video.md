# M2-01 API contract for libraries, items and playback sessions
Owner: mivo-backend
Depends on: none (library endpoints follow OD-1 of `M2-00-overview.md`, recommended option)
Files/areas: `api/openapi.yaml`, `api/spec_test.go`, `internal/adapters/http/apigen/**`
(regenerated + `oapi-codegen.yaml`), `internal/adapters/http/restapi/**` (new files
`library.go`, `playback.go`, `options.go`, `bodylimit.go` + tests; `api.go`, `problem.go`),
`internal/domain/errors.go` (+ test), `cmd/mivo/serve.go` (only the changed `restapi`
constructor call), `web/packages/api-client/src/generated/**` (regenerated),
`web/packages/api-client/src/{schemas.ts,index.ts,client.ts}` + tests, `go.mod`, `go.sum`
(oapi-codegen runtime for path parameters; the only go.mod change in wave 1)

## Goal
The whole milestone-2 API is specified first (ADR 0017, ADR 0022–0024), so backend and frontend
tasks can start in parallel against generated Go stubs and a generated TypeScript client.
Unimplemented operations answer a 501 problem until their task lands.

## Acceptance criteria
- [ ] `api/openapi.yaml` (OpenAPI 3.0.3, info.version `1.1.0`) adds, with operationIds,
      descriptions and examples:
      `GET /api/v1/libraries`, `GET /api/v1/libraries/{libraryId}`,
      `POST /api/v1/libraries/{libraryId}/scan` (202), `GET /api/v1/libraries/{libraryId}/items`
      (`limit` 1–500 default 100, opaque `cursor`), `GET /api/v1/items/{itemId}`,
      `POST /api/v1/playback/sessions` (201), `GET|DELETE /api/v1/playback/sessions/{sessionId}`,
      and the `stream` tag routes `GET|HEAD /api/v1/playback/sessions/{sessionId}/file` and
      `GET /api/v1/playback/sessions/{sessionId}/hls/{fileName}` (`fileName` pattern per ADR 0024).
- [ ] Schemas: `Library` (incl. `available`, `itemCount`, `scan`), `LibraryKind`
      (`movies|shows`), `ScanStatus` (`state` `idle|running|failed`, `startedAt`, `finishedAt`,
      counts `added updated removed unchanged failed skipped`, optional `errorCode` such as
      `root_unavailable`), `ItemSummary`,
      `MediaSummary`, `ItemPage`, `ItemDetail`, `MediaFile` (file *name* only, never a path),
      `MediaStream`, `ClientProfile` (ADR 0023, bounded `maxItems`/`maxLength`, identifier
      pattern), `PlaybackRequest`, `PlaybackSession` (incl. `state`
      `starting|ready|ended|failed` and an optional `failureCode` such as `transcode_failed`),
      `PlaybackMode`, `PlaybackDelivery`, `StreamDecision`, `PlaybackReason`. IDs are
      `format: uuid`. Enums are documented as extensible ("clients must tolerate new values").
- [ ] The `streamUrl` description states that clients must treat it as opaque (ADR 0024 auth
      readiness); stream routes have `security: []`.
- [ ] New problem codes documented in the `Problem.code` description and as `restapi`
      constants: `unsupported` (422), `unavailable` (503), `payload_too_large` (413),
      `no_compatible_format`, `transcode_unavailable` (422), `session_limit_reached`,
      `stream_not_ready` (503). `domain.ErrUnsupported` → 422 `unsupported` and
      `domain.ErrUnavailable` → 503 `unavailable` are added to the mapping table (table test).
- [ ] `oapi-codegen.yaml` excludes the two stream operations (`exclude-operation-ids`);
      `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean.
- [ ] `restapi.NewHandler` takes an options struct (system, library, scan and playback services,
      logger, extra `Mounts` of `{pattern, handler}` registered on the same mux). Nil feature
      services make their operations answer 501 with an internal `not_implemented` code (not a
      published code; removed in M2-32). Stub methods live in `library.go` / `playback.go`; the
      service interfaces there are placeholders that M2-26 / M2-28 fill in.
- [ ] Every API request body is limited to 64 KiB; a larger body → 413 `payload_too_large`
      problem (test with a 65 KiB body).
- [ ] An HTTP test binds a `uuid` path parameter (the note in `oapi-codegen.yaml`): valid UUID
      reaches the handler, `not-a-uuid` → 400 problem without internal text.
- [ ] A mounted test handler is reachable; wrong method on it → 405 problem.
- [ ] api-client exports the new schema types from `schemas.ts`/`index.ts`; BACKLOG row
      "api-client nits" is done (comment wording, `@throws` wording, negative scheme tests).
- [ ] `api/spec_test.go` asserts every `/api/v1/` path has an operationId and every
      non-stream operation a `default` problem response.
- [ ] Tests: restapi table tests for 501 stubs, body limit, uuid binding, mounts, error mapping;
      api-client Vitest for the nits.
- [ ] Gates: Go gates (lint, `go test -race ./...`, coverage gate), `pnpm -C web lint`,
      `pnpm -C web test`, generated-code drift check from `docs/development.md`.

## Notes
- Path parameters make the generated server import `github.com/oapi-codegen/runtime`; check that
  the `uuid` type mapping still binds (that is the point of the binding test).
- Keep handler stubs trivial; real behaviour comes in M2-26 (library), M2-28 (playback) and
  M2-29 (stream). Do not touch `internal/adapters/http/server` (M2-08 runs in wave 2).
- The BACKLOG item "truncateForLog before the first route with path parameters" is scheduled in
  M2-08 (wave 2), before any route gets a real implementation; stubs log through the existing
  `requestAttributes`, which M2-08 changes.
- If the library model changes through OD-1 (API-managed libraries), add
  `POST/DELETE /api/v1/libraries` here instead and report it.
