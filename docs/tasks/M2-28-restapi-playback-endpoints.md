# M2-28 REST API: playback sessions
Owner: mivo-backend
Depends on: M2-01, M2-08, M2-20 (runs in parallel with M2-25; only maps the session state)
Files/areas: `internal/adapters/http/restapi/playback.go` (+ new `playback_*.go` and tests),
`internal/adapters/http/restapi/problem.go` (playback mapping rows only)

## Goal
Clients create, read and stop playback sessions over the contract of M2-01, with precise problem
codes the player can translate (ADR 0023, ADR 0024).

## Acceptance criteria
- [ ] `PlaybackService` interface in restapi (consumer-declared) satisfied by
      `app/playback.Service`; the M2-01 stubs are replaced.
- [ ] `POST /api/v1/playback/sessions` → 201 with `Location` and the session; the client profile
      is validated (oversized arrays, unknown identifier shapes → 400 `invalid`).
- [ ] Error mapping (table test, specific before generic): unknown file 404 `not_found`;
      `playback.ErrNoCompatibleFormat` 422 `no_compatible_format`;
      `playback.ErrTranscodeUnavailable` 422 `transcode_unavailable`; session limit 503
      `session_limit_reached` with `Retry-After` (named constant seconds); file with probe error 422
      `unsupported`; shutting down 503 `unavailable`.
- [ ] `GET …/sessions/{sessionId}` → 200 or 404; `DELETE` → 204 always (idempotent).
- [ ] `streamUrl` is passed through from the app session unchanged; the paths are built by the
      URL builder of `internal/adapters/http/stream` (M2-29), which `cmd` hands to
      `app/playback` (M2-32). This task never formats stream paths itself.
- [ ] Reasons, warnings and per-stream decisions are mapped 1:1; unknown domain codes pass through
      as strings (clients tolerate new codes).
- [ ] Handler tests with a fake service for every status code and the JSON shape.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/http/...` + coverage gate.

## Notes
- Requests are cheap but start FFmpeg: log one info line per session start (mode, reasons, file
  ID, session ID) and per stop; never log the profile body at info.
- Do not edit `library.go` (M2-26) or `api.go`.
