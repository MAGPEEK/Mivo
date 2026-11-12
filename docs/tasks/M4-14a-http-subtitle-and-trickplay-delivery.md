# M4-14a HTTP: subtitle and trickplay stream routes, segment-request reporting
Owner: mivo-backend
Depends on: M4-02a, M4-02b, M4-13c, M4-13e
Files/areas: `internal/adapters/http/stream/subtitles.go`, `trickplay.go`, `hls.go` (+ tests)

## Goal
The player can download WebVTT and sprite sheets, and the HLS handler tells the service which segment was requested so the throttle works.

## Acceptance criteria
- [ ] `GET|HEAD /api/v1/media-files/{fileId}/subtitles/{trackId}.vtt`: `text/vtt; charset=utf-8`, `Cache-Control: private, max-age=3600`, `ETag`
      from the cache stamp, 304 on `If-None-Match`, Range via `ServeContent`; 404 unknown, 422 image format, 503 `Retry-After` when conversion is queued too long (20 s).
- [ ] `GET|HEAD /api/v1/media-files/{fileId}/trickplay/{sheet}.jpg` with `sheet` pattern `[0-9]{3}`; `image/jpeg`, `private, max-age=86400`.
- [ ] HLS segment requests call `StreamAccess.RequestSegment(index)` (M4-13b) for the throttle (index parsed from `seg-NNNNN.m4s`); playlist/init requests do not.
- [ ] Both routes honour the LAN guard of M2-31 and are logged at debug when successful.
- [ ] Tests: handler tests with fakes (headers, 304, Range, traversal-style track ids rejected by pattern), one corpus test.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Track id pattern `^[sx]:[0-9a-f]{1,40}$|^s:[0-9]{1,3}$`; reject everything else before any lookup.
