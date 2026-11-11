# M4-14b REST API: playback v2 fields, sessions list, transcoder, trickplay manifest
Owner: mivo-backend
Depends on: M4-02a, M4-02b, M4-13a, M4-13b, M4-13d, M4-13e
Files/areas: `internal/adapters/http/restapi/**` (+ tests), `internal/adapters/http/server/manageguard.go` (+ internal test)

## Goal
All strict-server operations of M4-02a/b are implemented and mapped.

## Acceptance criteria
- [ ] Session create maps `subtitleTrackId`, `maxBitRate`, returns subtitle decision, output and source bit rate; media-file resource lists tracks
      (embedded + external) and versions.
- [ ] The sessions list (`GET`/`HEAD /api/v1/playback/sessions`, exactly that path) and `/api/v1/system/transcoder[/test]` (202/409) are
      management-guarded (ADR 0028; non-local peer or non-admin → 403 `forbidden`), with guard-chain tests incl. encoded paths.
      `POST /playback/sessions` and `/playback/sessions/{sessionId}` stay open to every signed-in profile.
- [ ] Trickplay manifest `GET`/`POST` are no management operations: session + selected profile, same-origin on `POST` only, and a
      visibility check (a file the profile may not see → 404 `not_found`, like an unknown one).
- [ ] Domain errors map: `ErrInvalid` → 422, `ErrNotFound` → 404, `ErrBusy` → 409, `ErrUnavailable` → 503; no absolute path in any body.
- [ ] Tests: handler tests with fakes for every operation incl. guard and error mapping; response matches the OpenAPI schema (existing contract test).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- The guard exists since M3-19k; reuse, do not copy.
