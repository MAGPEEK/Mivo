# M6-01 API contract: identify, refresh, provider settings
Owner: mivo-backend
Depends on: M3-02
Files/areas: `api/openapi.yaml`, generated `internal/adapters/http/apigen`, `web/packages/api-client`

## Goal
The wire contract for Milestone 6 exists, so backend handlers (M6-13) and web dialogs (M6-15,
M6-16) can be built in parallel against it.

## Acceptance criteria
- [ ] `POST /api/v1/items/{id}/identify/search` (body: `query`, optional `year`, optional
      `providerId`/URL) returns candidates `{provider, externalId, title, year, overview,
      posterUrl (server image URL, not a provider URL), confidence}`.
- [ ] `POST /api/v1/items/{id}/identify` (body: `provider`, `externalId`) applies a match;
      `DELETE /api/v1/items/{id}/match` removes it (state `disabled`);
      `POST /api/v1/items/{id}/refresh` and `POST /api/v1/libraries/{id}/refresh` queue a
      refresh and return `202`.
- [ ] Item DTOs gain `matchState` (`matched|unmatched|disabled`) and `metadataSources`
      (list, e.g. `["nfo","tmdb"]`); the items list accepts `matchState=unmatched`.
- [ ] `GET/PUT /api/v1/settings/metadata`: `onlineEnabled`, per provider `enabled`, TMDB key
      status `none|bundled|own` (key is write-only, never returned), `language`; plus
      `GET /api/v1/metadata/status` with queue length, per-provider state `ok|rate_limited|
      offline|disabled`.
- [ ] Error mapping documented: `404` unknown item, `409` online disabled, `503` provider
      unavailable (problem+json with `retryAfterSeconds`).
- [ ] Management endpoints are marked with the ADR 0028 guard note.
- [ ] Tests: `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean; the
      OpenAPI lint step passes.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Candidate poster URLs must point at the Mivo image endpoint or a short-lived server proxy
route, never directly at `image.tmdb.org`, so the browser stays free of third-party hosts
(ADR 0010). Stubs may return `501` until M6-13.
