# M3-11 Watcher and periodic scan (config keys)
Owner: mivo-backend
Depends on: M3-01 (uses the existing `app/scan.Service`; independent of scan v2)
Files/areas: `internal/adapters/fs/watcher/**` (new), `internal/app/scantrigger/**` (new),
`internal/platform/config/**` (`settings.go`, `config.go`, tests),
`docs/adr/0018-configuration-and-os-paths.md` (a new "Notes (M3-11)" section only), `go.mod`/`go.sum` (`github.com/fsnotify/fsnotify`, the
only `go.mod` change of wave 1)

## Goal
New, changed and removed files appear without pressing "Scan": a file system watcher triggers
a debounced scan of the affected library, and every library is rescanned periodically, which
also covers network shares that send no events. UX: **AD-23** (config keys for watcher and
interval); ADR 0027 §9, OD-M3-3.

## Acceptance criteria
- [ ] Config keys `scan.watch` (`<PREFIX>_SCAN_WATCH`, default `true`, strict booleans like
      `server.lan_only`) and `scan.interval` (`<PREFIX>_SCAN_INTERVAL`, Go duration, default
      `12h`, `0` disables, otherwise 15m–168h); errors name the key or variable; documented in
      a "Notes (M3-11)" section of ADR 0018 with the table format of M2-04.
- [ ] `watcher` adapter: `Watch(ctx, root string, changed func()) error` watches the root
      recursively (adds watches for new directories, skips the NAS system folders and hidden
      entries of ADR 0022), calls `changed` for create/write/remove/rename events of library
      files and directories, returns `domain.ErrUnavailable` when the root cannot be watched,
      and reports a watch-limit failure (inotify `ENOSPC`, Windows buffer overflow) through an
      error callback without stopping the other watches. Ends when ctx is done.
- [ ] `scantrigger` use case: per library a debounce (quiet 30 s, max 5 min after the first
      event; named constants) that calls the scanner's `Start`; an event during a running
      scan schedules exactly one more scan after it finishes; a periodic check (every minute)
      starts a scan for each library whose last scan finished more than `interval` ago;
      `interval = 0` and `watch = false` disable the respective part. One warning per library
      when watching fails ("relying on the periodic scan").
- [ ] Ports: library list, scanner (`Start`, `Status` – adapt `app/scan.Service` directly,
      app may import app), watcher, clock/timer (injectable for tests). `Run(ctx)` starts
      everything and returns when ctx is done; no goroutine survives it.
- [ ] Tests: use case with fake clock and fake watcher (debounce, max delay, coalescing during
      a running scan, periodic due/not due, disabled modes, watcher failure logs once, clean
      shutdown); watcher adapter against a temp directory (create, nested new directory,
      rename, removal, system folder ignored) on Windows and in Docker for Linux; config
      table tests for both keys incl. env overrides and bounds.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/adapters/fs/watcher/... ./internal/app/scantrigger/...
      ./internal/platform/config/...`, coverage gate, `go mod tidy -diff`,
      `go tool "-modfile=tools/go.mod" govulncheck ./...` [net].

## Notes
- Wiring (`cmd/mivo`) is M3-11b; this task must not touch `cmd/mivo`.
- fsnotify is not recursive on Linux: walk the tree once with the same confinement rules as
  the library walker (only real directories inside the root; follow no links) and add one
  watch per directory; on Windows the recursive mode of fsnotify may be used if it is stable
  in the pinned version – document the choice.
- A scan triggered by the watcher is a normal full scan (ADR 0027 §9); partial scans are a
  later optimisation.
- Network approval for the module download is needed (`github.com/fsnotify/fsnotify`).
