# M7-14 App: viewer-scoped catalog
Owner: mivo-backend
Depends on: M7-13, M7-15
Files/areas: `internal/app/access/` (scoped library source), `internal/app/library/` (incl. `ports.go`, `folder_view.go`), `internal/app/rows/`, `internal/app/search/`, `internal/app/playback/` (start check), their tests

## Goal
Every catalog use case reads the viewer from the context and passes its restriction, so
enforcement happens on the server for every list, row, search, detail, folder view and stream
start (US-12), including the ancestor rule of ADR 0032 §7 (amendment 2026-11-11).

## Acceptance criteria
- [ ] `app/access.ScopedLibraries` replaces `rows.AllLibraries`: visible libraries are the
      viewer's effective access set; an inaccessible library id wraps `domain.ErrNotFound`.
- [ ] `app/library`, `app/rows`, `app/search` build the restriction from `access.ViewerFrom(ctx)`
      and remove every `TODO(M7-14)`; no viewer → `domain.ErrUnauthenticated` (fail closed).
- [ ] **Denied library first.** Every use case that takes a library id (listing, facets,
      letters, folder view, rows of a library) checks the viewer's access before any other
      validation and before any store or file-system call; a library outside the access set
      wraps `domain.ErrNotFound`, also when limit, cursor, filter or path are invalid (test:
      denied library with an invalid cursor → `ErrNotFound`, not `ErrInvalid`; the fakes
      record no call).
- [ ] **Folder view.** `FolderItemReader` gains
      `ListFolderDirectories(ctx, rootID, directory, restriction)` (implemented in sqlite by
      M7-13) and `ListFolderItems` passes the restriction. The entries of a directory are the
      file-system directories **intersected** with the catalog's visible ones, so a directory
      that holds only hidden items (for example the folder of a hidden show with a
      lower-rated episode) is absent; opening such a directory by path wraps
      `domain.ErrNotFound` like a missing one. An unrestricted viewer (admin) still sees
      every file-system directory, also empty ones.
- [ ] Starting a playback session for a hidden item or a file of a hidden item answers
      `domain.ErrNotFound`, also when only an ancestor is hidden; a session records the auth
      session id that started it, and get, stop, file and HLS requests of another auth session
      (non-admin) answer `ErrNotFound`.
- [ ] Background jobs (scan, enrichment, scan trigger) do not depend on a viewer (test with a
      bare context).
- [ ] Tests: use-case tests with in-memory fakes (`go test ./internal/app/...`): limited viewer,
      admin viewer, missing viewer, library outside access (each use case), folder view with a
      hidden-only directory and with a mixed directory, direct path into a hidden directory,
      session ownership.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
The playback ownership check refines ADR 0024: add a short Notes (M7-14) section there. Images
stay unscoped (ADR 0032 §7). Keep each service change small; if this exceeds ~400 lines, split
playback into M7-14b and tell the orchestrator.

Folder view: today `listFolder` validates limit and cursor before it loads the library; move
the access check to the front. The intersection is by directory name inside one library
folder; compare names exactly as the catalog stores path segments (no case folding, the
file system decides). The top level (library folders of an accessible library) is unchanged.
The in-memory fake of `FolderItemReader` must implement the ancestor rule itself, or the test
proves nothing. Also open from backlog "m7-18 review minors": restriction in the watch
episode counts and `EpisodeLister.ListEpisodes`.
