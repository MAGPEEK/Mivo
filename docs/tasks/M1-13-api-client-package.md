# M1-13 api-client package generated from the OpenAPI spec
Owner: mivo-frontend
Depends on: M1-07, M1-11
Files/areas: `web/packages/api-client/**`

## Goal
Web code talks to the server only through a typed client generated from
`api/openapi.yaml` (ADR 0017). Errors arrive as one normalized type.

## Acceptance criteria
- [ ] The package script `gen` runs openapi-typescript on `../../../api/openapi.yaml` and
      writes `src/generated/schema.ts`. `pnpm -C web gen` (root delegate) runs it. A second
      run followed by `git diff --exit-code web/packages/api-client/src/generated` is clean.
- [ ] Generated output is committed, has the generator's "do not edit" header and is
      excluded from ESLint, Prettier and coverage.
- [ ] `createApiClient({ baseUrl, fetch? })` returns an openapi-fetch client typed with the
      generated `paths`. `baseUrl` defaults to `''` (same origin).
- [ ] Exported type aliases: `HealthResponse`, `SystemInfo`, `Problem` (from
      `components['schemas']`).
- [ ] `ApiError` (status, title, code, detail) and a helper that turns an openapi-fetch
      error result into `ApiError`. A problem+json body is used when present; network
      failures and non-problem bodies get a fallback code (e.g. `network_error`,
      `unexpected_response`).
- [ ] No React or TanStack dependency (dependency-cruiser rule).
- [ ] Tests (fake `fetch`, no network): `GET /api/v1/system/info` returns typed data; 503
      from `/api/v1/health` returns the typed body; a problem+json 404 becomes `ApiError`
      with its `code`; a rejected fetch becomes `ApiError` `network_error`.
- [ ] Gates: `pnpm -C web lint` and `pnpm -C web test` (thresholds) pass.

## Notes
- Pin `openapi-typescript` and `openapi-fetch` versions exactly. Generator upgrades are
  deliberate (ADR 0017).
- Keep the entry point small (`src/index.ts`). dependency-cruiser forbids deep imports into
  `src/generated` from outside the package.
- The TV app will reuse this package with a different `baseUrl`. Never read
  `window.location` here.
