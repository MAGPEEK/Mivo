# 0002 "Mivo" is a working title with a single configurable source

- Status: Accepted
- Date: 2026-10-29
- Source: decision table in `docs/plan.md`

## Context

"Mivo" collides with the Indonesian app "Mivo TV". The final name is chosen before the first
public release, so renaming must be cheap.

## Decision

- User-visible names and name-derived values come from exactly two modules:
  - Go: `internal/platform/brand` (display name, lowercase id, env prefix, repository URL)
  - Web: `web/packages/ui-tokens/src/brand.ts` (product name, id, repository URL)
- Everything else derives from them: UI strings, page title, log fields, OS directory names,
  environment variable prefix, User-Agent, storage keys.
- Code identifiers never contain the name (`createApiClient`, not `createMivoClient`).
- Some **structural identifiers** cannot read a constant. They may contain the working title
  and form the rename checklist:
  1. Go module path (ADR 0016) and the `cmd/mivo` directory / binary name
  2. npm workspace scope `@mivo/*`
  3. `api/openapi.yaml` `info.title`
  4. Docker image name, the `MIVO_*` variables and the OCI labels in `packaging/docker/`
  5. Later: webOS app id, GoReleaser config, installer names, systemd unit

## Consequences

- A rename means editing two brand files plus the checklist above.
- Reviewers reject new hard-coded occurrences. A quick check is a case-sensitive search for
  `Mivo` in `*.go`, `*.ts` and `*.tsx` files: the only expected hits are the brand modules and
  tests that assert on them.
