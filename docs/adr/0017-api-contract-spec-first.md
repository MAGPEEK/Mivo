# 0017 Spec-first REST API with generated Go stubs and TypeScript client

- Status: Proposed
- Date: 2026-10-29
- Refines: tech stack row "API" in `docs/plan.md`

## Context

The web app, the webOS app and third-party scripts all consume the API. Hand-written clients
drift from the server.

## Decision

- `api/openapi.yaml` is the single source of truth. It uses **OpenAPI 3.0.3**, because
  oapi-codegen supports 3.0 fully and 3.1 only partially.
- Paths carry the full prefix (`/api/v1/...`). `servers` is `/`. JSON fields use camelCase.
- Errors use **RFC 9457 problem details** (`application/problem+json`) with `type`, `title`,
  `status`, `detail` and a machine-readable `code` that clients translate (ADR 0012). Domain
  errors are mapped to HTTP status codes only in `internal/adapters/http`.
- Go: **oapi-codegen v2** (`std-http-server`, `strict-server`, `models`), pinned as a
  `go tool` dependency in `go.mod`. Output goes to `internal/adapters/http/apigen` through
  `go generate`.
- TypeScript: **openapi-typescript** generates types into `web/packages/api-client`. The
  runtime client is **openapi-fetch**. TanStack Query hooks live in the app's feature
  folders, not in the client package.
- Generated code is committed with its `DO NOT EDIT` header. It is excluded from lint and
  coverage and never edited by hand. The drift check is: `go generate ./...` and
  `pnpm -C web gen` must leave `git diff --exit-code` clean.
- Compatibility: within `/api/v1` only additive changes are allowed. Breaking changes need a
  new version. WebSocket events are documented separately later.

## Consequences

- Server and clients stay in lockstep through compile errors instead of runtime surprises.
- Generator upgrades are deliberate changes (version pinned in `go.mod` and
  `package.json`).
