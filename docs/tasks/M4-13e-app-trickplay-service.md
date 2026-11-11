# M4-13e App: trickplay generation and lookup
Owner: mivo-backend
Depends on: M4-12, M4-02b
Files/areas: `internal/app/trickplay/**` (new package, + tests)

## Goal
Seek previews are generated in the background when wanted and never compete with a viewer's transcode (ADR 0030 §8).

## Acceptance criteria
- [ ] `Manifest(ctx, fileID)` reads `<cache>/trickplay/<fileID>/manifest.json` or `ErrNotFound`; `Request(ctx, fileID)` enqueues generation and returns at once.
- [ ] One job at a time; a job starts only when `playback.Service.List` shows no running re-encode (or `max_sessions > 1`), polls every 30 s, gives up after 1 h.
- [ ] Output is keyed with the file's size+mtime; a changed file regenerates; cache entries of vanished files are removed at startup.
- [ ] Failures are remembered for the process lifetime (no retry loop, also not on a repeated `Request`) and logged once at warn;
      direct-play-only / missing FFmpeg → `ErrUnavailable`.
- [ ] Requests are de-duplicated per file (queued, running or done → no second job); pending jobs are capped (named constant), a full
      queue drops the request without an error (the contract still answers 202).
- [ ] `<cache>/trickplay` is capped in size (named constant or expert key) with eviction of the least recently used entries.
- [ ] Tests: fakes for generator, clock and session lister: queueing, de-duplication, full queue, eviction, busy server delays, stale key, failure memory, cancellation on shutdown.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- No generation during scans; the scan never calls this package.
- Disc images and files without a video stream are `ErrInvalid` (no entry).
