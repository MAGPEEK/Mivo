# M4-02b API contract: session list, hardware acceleration, trickplay
Owner: mivo-backend
Depends on: M4-02a
Files/areas: `api/openapi.yaml`, generated code (apigen, api-client), spec test

## Goal
Session management, the hardware-acceleration status/test and trickplay are in the contract
before any handler exists (ADR 0030 §5, §7, §8; AD-25, PL-05, PL-41).

## Acceptance criteria
- [ ] `GET /playback/sessions` returns active sessions: `id`, `itemId`, `title`, `state`, `mode`,
      `videoEncoder`, `hardware` (bool), `outputBitRate`, `transcodeSpeed` (float, 1.0 = real time),
      `paused` (throttled), `clientName`, `startedAt`; management-guard error documented (403).
- [ ] `GET /system/transcoder` returns `{hwaccel: auto|off|..., devices: [{api, encoder, codec, usable,
      degraded, message}], throttleSupported, lastTestedAt}`; `POST /system/transcoder/test` re-runs
      the test run (202 with the new status or 409 while a run is active). Both management-guarded.
- [ ] `GET /media-files/{fileId}/trickplay` returns manifest `{tileWidth, tileHeight, columns, rows,
      intervalSeconds, sheetCount}` or 404 (not generated; a `POST` with 202 requests generation);
      `GET .../trickplay/{sheet}.jpg` is a stream route like the subtitle route.
- [ ] Generated code clean; spec test extended.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Sessions list must not leak absolute paths; `title` comes from the catalog.
- `transcodeSpeed` and `paused` are optional (absent for direct play).
