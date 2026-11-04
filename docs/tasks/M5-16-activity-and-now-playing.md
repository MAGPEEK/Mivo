# M5-16 Activity and now-playing (read-only)
Owner: mivo-backend
Depends on: M3-11
Files/areas: `api/openapi.yaml`, generated code, `internal/app/` (activity aggregation), `internal/adapters/http/`, `cmd/mivo`

## Goal
The client can show the activity indicator (SH-04: scans with progress, active transcodes) and
the admin dashboard "Now playing" (AD-10, PL-43) from one read-only API.

## Acceptance criteria
- [ ] `GET /api/v1/activity` returns running scans (library, progress) and playback sessions
      (item title, delivery mode and reason codes as in the playback API, client address class
      LAN/remote, FFmpeg process CPU % when known) as lists; empty lists when idle.
- [ ] Polling-friendly (`ETag` or small payload); an SSE/WebSocket stream is an extension, not
      required (SH-04 allows polling).
- [ ] Management guard of ADR 0028 applies to session details (local peers) until M7.
- [ ] Tests: use case with fakes (scan + session sources), handler tests incl. guard.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Define consumer-side ports `ScanActivity` and `SessionActivity` (1-2 methods each); reuse
existing scan state and session registry, do not duplicate state.
