# M3-19j Scan triggers: dynamic library set, one watch per folder
Owner: mivo-backend
Depends on: M3-19b, M3-11b
Files/areas: `internal/app/scantrigger/**`

## Goal
Libraries added, changed or removed in the app are watched and periodically rescanned without
a server restart, with one file system watch per library folder.

## Acceptance criteria
- [ ] `scantrigger` implements the `LibraryEvents` port of M3-19e (`Added`, `Changed`,
      `Removed` with a library id); the library list port returns M3-19b `Library` values with
      folders.
- [ ] Added: watches for every folder start and the library joins the periodic check.
      Changed: watches of removed folders stop, watches of new folders start, debounce state is
      kept. Removed: all its watches stop, pending debounces are dropped, the periodic check
      forgets it. Events for unknown ids are ignored with a debug log.
- [ ] A watch failure is logged once per folder (not per library) with the folder path, and the
      library relies on the periodic scan for that folder.
- [ ] No goroutine outlives `Run(ctx)`, also after many add/remove events (test with a leak
      check or counting fakes).
- [ ] Tests with the fake clock and fake watcher of M3-11: add during run, change folders,
      remove during a pending debounce (no scan starts), remove during a running scan (no extra
      scan scheduled), per-folder failure logged once, shutdown.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/app/scantrigger/...`, coverage gate.

## Notes
- Events may arrive from HTTP goroutines; hand them to the trigger's loop through a channel
  instead of locking shared maps from several goroutines.
- `cmd` wiring is M3-19l.
