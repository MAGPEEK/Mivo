# M4-13c App: subtitle service with bounded cache
Owner: mivo-backend
Depends on: M4-08a, M4-06b, M4-02a
Files/areas: `internal/app/subtitles/**` (new package, + tests)

## Goal
The web player fetches a WebVTT for any text track of a file; conversion happens once and is cached (ADR 0030 §3).

## Acceptance criteria
- [ ] `Service.WebVTT(ctx, fileID, trackID) (ReadSeekCloser-like handle, modTime, error)` resolves the track (embedded or external), converts
      through port `Converter` (named so in the code), stores under `<cache>/subtitles/<fileID>/<sha1(trackID)[:16]>-<sha256(sourceStamp)[:16]>.vtt` (digest names: track ids like `s:2`/`x:...` and stamps are not valid Windows file names; amended by the orchestrator 2026-11-12), serves the cached file on repeat.
- [ ] `sourceStamp` = media file size+mtime (+ external file size+mtime): a changed source misses the cache.
- [ ] Concurrent requests for the same track convert once (singleflight); at most 2 conversions at a time (weak NAS).
- [ ] Image formats → `domain.ErrInvalid`; unknown track → `ErrNotFound`.
- [ ] Cache is purged at startup of entries of unknown files and capped at 64 MiB (oldest first).
- [ ] Tests: fakes for converter and file source; cache hit/miss, stale stamp, singleflight, cap eviction.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- Path layout is app-owned; the FFmpeg adapter only receives a destination path (M4-08a).
- No library root escapes: external files come through the same `PathResolver` as media.
