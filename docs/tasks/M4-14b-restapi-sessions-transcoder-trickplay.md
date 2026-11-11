# M4-14b REST API: playback v2 fields, sessions list, transcoder, trickplay manifest
Owner: mivo-backend
Depends on: M4-02a, M4-02b, M4-13a, M4-13b, M4-13d, M4-13e
Files/areas: `internal/adapters/http/restapi/**` (+ tests)

## Goal
All strict-server operations of M4-02a/b are implemented and mapped.

## Acceptance criteria
- [ ] Session create maps `subtitleTrackId`, `maxBitRate`, returns subtitle decision, output and source bit rate; media-file resource lists tracks
      (embedded + external) and versions.
- [ ] `GET /playback/sessions`, `GET /system/transcoder`, `POST /system/transcoder/test` (202/409), trickplay manifest `GET`/`POST` work behind
      the ADR 0028 management guard (non-local peer → 403 `forbidden`).
- [ ] Domain errors map: `ErrInvalid` → 422, `ErrNotFound` → 404, `ErrBusy` → 409, `ErrUnavailable` → 503; no absolute path in any body.
- [ ] Tests: handler tests with fakes for every operation incl. guard and error mapping; response matches the OpenAPI schema (existing contract test).
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- The guard exists since M3-19k; reuse, do not copy.
