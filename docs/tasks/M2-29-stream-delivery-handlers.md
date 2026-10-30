# M2-29 HTTP adapter: direct file and HLS delivery
Owner: mivo-backend
Depends on: M2-01, M2-05, M2-08, M2-12, M2-20 (uses `Access` and the ready wait; runs in
parallel with M2-25)
Files/areas: `internal/adapters/http/stream/**` (new package)

## Goal
The bytes reach the browser: the original file with Range support for Direct Play, and the
session's playlist, init segment and media segments for HLS – confined to their directories and
ready for signed URLs later (ADR 0024).

## Acceptance criteria
- [ ] The package returns `restapi` mounts for `GET /api/v1/playback/sessions/{sessionId}/file`
      and `GET /api/v1/playback/sessions/{sessionId}/hls/{fileName}`, plus the URL builder that
      `cmd` hands to `app/playback` (the single place that knows these paths; it returns the
      direct URL or the `…/hls/index.m3u8` URL for a session ID and delivery).
- [ ] Both handlers call the playback service's `Access` (records the access time); unknown or
      stopped session → 404 problem; malformed session ID → 400.
- [ ] Direct file: opened through the port implemented by `libraryfs.FileSystem.Open` (os.Root), served with `http.ServeContent`
      (Range, `If-Range`, HEAD, 206/416) and the ADR 0023 `Content-Type`; tests for full, range,
      suffix range, invalid range (416), HEAD, and that the response writer chain still exposes
      `io.ReaderFrom` (M2-08).
- [ ] HLS: `fileName` must match `index.m3u8|init.mp4|seg-[0-9]{5}.m4s`, else 404 (tests with
      `../x`, `%2e%2e/x`, `INDEX.M3U8`, `seg-1.m4s`); files opened through `os.Root` of the
      session directory; explicit content types and cache headers per ADR 0024.
- [ ] Playlist requests wait for readiness up to 20 s (named constant, cancelled with the request);
      on timeout 503 `stream_not_ready` with `Retry-After`; a failed production → 500 problem
      logged with the session ID.
- [ ] Integration test with a real producer on the MKV corpus clip (skip without FFmpeg/corpus):
      playlist → init → first segment all 200 with correct types.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/http/...` + coverage gate.

## Notes
- No auth in milestone 2 (ADR 0024 readiness section); keep the handlers free of assumptions
  about the URL shape beyond the builder, so a token path segment can be added in milestone 7.
- `http.ServeContent` needs a `ReadSeeker` and a modtime; use the file's mtime, no ETag.
- Never write a problem after the body has started (use the statuswriter recorder).
- (from M2-08 review r2) On quiet stream routes a failure after the 200 is sent (e.g. FFmpeg dies
  mid-segment) would be logged at debug as status 200 if the handler just returns. Stream handlers
  must log such failures themselves at warn or `panic(http.ErrAbortHandler)` so the request is
  recorded as aborted at info.
