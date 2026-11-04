# M3-09 Images: use case and disk cache
Owner: mivo-backend
Depends on: M3-04b
Files/areas: `internal/app/images/**` (new), `internal/adapters/fs/imagecache/**` (new)

## Goal
Requesting an artwork in an allowed width returns a cached rendering or renders it once,
bounded in concurrency and time, and the cache never holds files for artwork that no longer
exists. The HTTP endpoint (M3-09b) and the composition root (M3-11b) only connect this to the
outside. UX: LB-44; ADR 0027 §8.

## Acceptance criteria
- [ ] Ports declared in `app/images`: artwork source (`Artwork(ctx, id) (artwork.Stored,
      error)`, `ArtworkTags(ctx) (map[string]string, error)`), path resolver (`Resolve(root,
      relativePath)`, same contract as the scan's), renderer (`Render(ctx,
      artwork.RenderRequest) (artwork.Rendered, error)`), cache (`Open(ctx, key)`,
      `Store(ctx, key, data)`, `Purge(ctx, keep func(id, tag string) bool)`).
- [ ] `Service.Image(ctx, Request{ID, Width artwork.Width, Tag string})` returns
      `Image{Content io.ReadSeekCloser, Size, ContentType, ETag, Immutable bool}`: unknown ID
      → `ErrNotFound`; cache hit served without rendering; miss → resolve (confined to the
      library root) → render with `MaxWidth = width` and `artwork.OutputFormat(kind)` → store
      → serve. `Immutable` is true only when `Tag` equals the stored tag (ADR 0027 §8).
- [ ] At most two renders at a time across the service (named constant); concurrent requests
      for the same `(id, tag, width)` share one render (singleflight-style, no new
      dependency); a waiting request honours `ctx`.
- [ ] Renderer errors pass through classified (`ErrUnsupported` → 422 later, `ErrUnavailable`
      → 503); a failed render stores nothing.
- [ ] `Service.Purge(ctx)` keeps only current `(id, tag)` pairs; it is safe to run while
      images are served.
- [ ] `imagecache`: layout `<cache>/images/<id[0:2]>/<id>/<tag>-<width>.<ext>`, IDs and tags
      validated before they become path parts (UUID text, lowercase hex), write to a temp
      file in the same directory and rename, `Open` of a missing entry → `ErrNotFound`,
      `Purge` removes stale files and empty directories and ignores foreign files (logs them
      at debug). Works on Windows (retry rename/remove like `transcodedir`, without copying
      its code into a shared helper in this task).
- [ ] Tests: use case with fakes (hit, miss, tag mismatch, concurrency limit observed by a
      fake renderer, same-key sharing, ctx cancel while waiting, purge); cache adapter
      against a temp dir (atomic write, traversal attempts with crafted IDs rejected,
      purge). Coverage ≥ thresholds.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/images/... ./internal/adapters/fs/imagecache/...`,
      coverage gate.

## Notes
- Embedded artwork resolves the media file's relative path, then renders with the stream
  index; data-directory sources (M6) need a second resolver later – keep the source switch in
  one function.
- The cache directory comes from `paths.cache` (ADR 0018); the adapter gets the absolute
  `images` directory from cmd and creates it on first use with 0750.
- No HTTP code here; `net/http` is forbidden in `app` (ADR 0019).
