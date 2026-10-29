# M1-07 OpenAPI skeleton and generated Go server stubs
Owner: mivo-backend
Depends on: M1-02, M1-03
Files/areas: `api/openapi.yaml`, `api/doc.go`, `api/spec_test.go`,
`internal/adapters/http/apigen/**` (config, `generate.go`, generated file), `go.mod`, `go.sum`

## Goal
The API contract exists as the single source of truth (ADR 0017), with the first two
operations. Go server interfaces and models are generated from it.

## Acceptance criteria
- [ ] `api/openapi.yaml` is OpenAPI 3.0.3. `info.title` is "Mivo API" (rename checklist item,
      ADR 0002). `servers: [{url: /}]`. Tag `system`.
- [ ] `GET /api/v1/health` (`operationId: getHealth`): `200` and `503`, both with
      `HealthResponse` `{status: enum[ok, unavailable], checks: [{name, status, detail?}]}`.
- [ ] `GET /api/v1/system/info` (`operationId: getSystemInfo`): `200` with `SystemInfo`
      `{productName, version, commit?, buildTime? (date-time), serverId (format uuid),
      apiVersion}` and a `default` response with `Problem`.
- [ ] `components.schemas.Problem` follows RFC 9457 (`type`, `title`, `status`, `detail`,
      `instance`) plus a required machine-readable `code`. Media type is
      `application/problem+json`. A reusable `components.responses` entry references it.
- [ ] `api/spec_test.go` loads and validates the spec with kin-openapi
      (`openapi3.NewLoader` + `Validate`). `go test ./api/...` passes.
- [ ] oapi-codegen v2 is pinned with `go get -tool`. `internal/adapters/http/apigen/generate.go`
      holds the `//go:generate go tool oapi-codegen -config ...` line. The config enables
      `std-http-server`, `strict-server` and `models`.
- [ ] `go generate ./internal/adapters/http/apigen/...` followed by
      `git diff --exit-code internal/adapters/http/apigen` is clean (generated code committed
      and current, with `DO NOT EDIT` header).
- [ ] The generated `StrictServerInterface` contains `GetHealth` and `GetSystemInfo`.
      `go build ./...` passes.
- [ ] Tests: spec validation test (above).
- [ ] Gates: `golangci-lint run` (generated file excluded), `go test ./...` and the coverage
      check pass (`apigen` excluded from coverage).

## Notes
- `api/doc.go` (`package api`) exists so the spec test has a package. Do not embed or serve
  the spec yet (Swagger UI at `/api/docs` comes later).
- The 503 response must be declared, or the strict server cannot return it.
- Write descriptions for every schema property. The TS client (M1-13) and future API users
  rely on them.
- M1-13 (frontend) generates the TypeScript client from this file. After this task is merged,
  spec changes go through new tasks.
