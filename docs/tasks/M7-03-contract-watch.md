# M7-03 Contract: watch state, progress, viewer state, row kinds
Owner: mivo-backend
Depends on: M7-02
Files/areas: `api/openapi.yaml`, generated `internal/adapters/http/apigen`, `web/packages/api-client`, new `internal/adapters/http/restapi/watch_stub.go`

## Goal
Clients can report progress, mark items watched and read per-profile state on items and rows,
so the watch handlers (M7-23) and the player/detail/home work (M7-35…M7-37) proceed in parallel.

## Acceptance criteria
- [ ] Tag `watch`: `PUT /api/v1/items/{itemId}/progress` (`positionSeconds` ≥ 0, optional
      `playbackSessionId`) → `204`; `PUT /api/v1/items/{itemId}/watched` and
      `DELETE /api/v1/items/{itemId}/watched` → `204` (shows and seasons apply to their
      episodes); `DELETE /api/v1/items/{itemId}/continue-watching` → `204` (hide).
      `400 invalid` for kinds without watch state (only movie, episode, audiobook, and show/season
      for `watched`), `404` for unknown or hidden items.
- [ ] Optional `viewerState` on item summaries and item detail: `watched`, `positionSeconds`,
      `lastPlayedAt`, `playCount`, and `unwatchedCount` for shows and seasons. Absent means
      "no state" (and for kinds without watch state).
- [ ] `RowKind` gains `continueWatching` and `upNext` (description updated); rows with these
      kinds use the existing `Row` shape; episode cards carry what `S3 · E9` needs (reuse the
      existing summary fields or add `seasonNumber`/`episodeNumber`/`showTitle` if missing).
- [ ] `watch_stub.go` answers `501` for the new operations.
- [ ] Tests: `go generate ./...` and `pnpm -C web gen` leave `git diff --exit-code` clean;
      OpenAPI lint passes; `go test ./internal/adapters/http/...` green.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Units in seconds like `startPositionSeconds` (playback). ADR 0032 §8 for the rules. Progress is
idempotent (PUT), so `keepalive` retries are harmless.
