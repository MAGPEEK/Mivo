# M3-19c Config: browse roots, install kind, libraries as import seed
Owner: mivo-backend
Depends on: M3-11 (owns `internal/platform/config` in wave 1), ADR 0028 written
Files/areas: `internal/platform/config/**` (`settings.go`, `config.go`, a new `browse.go`,
tests), `docs/adr/0018-configuration-and-os-paths.md` (a new "Notes (M3-19c)" section only),
`docs/development.md` (section "Libraries and tools" only)

## Goal
The server knows where the folder browser may start and what kind of installation it runs in,
and the configuration documents that configured libraries are imported once and then managed
in the app. (The Docker image switches to the new keys in M3-19l, once the management API
works end to end.)

## Acceptance criteria
- [ ] Key `browse.roots` (YAML list of paths) / `<PREFIX>_BROWSE_ROOTS` (OS path list,
      `filepath.SplitList`; replaces the YAML list as a whole; an empty value means no roots).
      Paths follow the ADR 0018 path rules (relative → config directory, `~` and
      drive-relative refused), must be absolute and unique after cleaning (case-insensitive on
      Windows); a missing directory is **not** an error (reported unavailable later).
- [ ] Defaults when the key is absent: Windows – every present drive root (`A:\` … `Z:\` that
      a stat finds, evaluated at startup); others – those of `/media`, `/mnt`, `/srv` and the
      home directory (from the injected environment) that exist. The default computation takes
      injected functions (stat, environment), so it is testable on any OS.
- [ ] Key `install.kind` / `<PREFIX>_INSTALL_KIND`: `standard` (default), `docker`, `synology`,
      case-insensitive; anything else is a startup error naming the key or variable.
- [ ] `libraries` / `<PREFIX>_LIBRARIES` keep their syntax and validation; their doc comments,
      the ADR 0018 Notes and `docs/development.md` say they are an **import seed**: imported
      once at startup, then managed in Settings › Libraries; removing an entry deletes nothing
      (M3-19 Notes "Persistence").
- [ ] "Notes (M3-19c)" in ADR 0018 with the table format of M2-04 (key, environment, default,
      validation) for both keys and one paragraph on the seed semantics of `libraries`.
- [ ] Tests: config table tests for both keys (file, env override, empty env, duplicates incl.
      Windows case, relative paths, bad values), defaults with fake stat/env for both OS
      families.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./internal/platform/config/...`, coverage gate.

## Notes
- `platform/config` must not import the domain (ADR 0018); values reach `cmd` as plain strings
  and slices. `cmd` is not touched here (M3-19l wires the keys).
- Browse roots may contain the data or transcode directory (on DSM `/volume1` contains
  `@appdata`); the browser hides and the domain refuses those (M3-19b, M3-19d), so no overlap
  rule applies to browse roots in the configuration.
- On Synology the package's start script sets both variables (SPK-17); the environment wins
  over `config.yaml` (ADR 0018), as for the package's paths.
