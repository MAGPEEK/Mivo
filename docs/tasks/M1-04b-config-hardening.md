# M1-04b Harden configuration loading
Owner: mivo-backend
Depends on: M1-04
Files/areas: `internal/platform/config/**`, `internal/platform/paths/**`, `docs/adr/0018-configuration-and-os-paths.md` (Notes only)

## Goal
Resolve the M1-04 review findings so invalid configuration never silently falls back to defaults
and errors are clear for the startup message in M1-10.

## Acceptance criteria
- [ ] `config` uses `logging.Level*` / `logging.Format*` constants (defaults and validation); the
      duplicated `config.LogLevel*` / `LogFormat*` constants are removed.
- [ ] An explicit YAML null (`paths:\n  data:` or `level: ~`) is a startup error naming the key and
      file (decode leaves as `yaml.Node`: missing vs `!!null`). Tests for both.
- [ ] A second YAML document (`---`) is rejected with a clear "multiple documents" error. Test.
- [ ] Relative `LOCALAPPDATA` is ignored like relative XDG values; a relative home dir counts as
      unknown. `Config.Paths` are always absolute (test).
- [ ] Unknown-key errors are one line: `<file>: line N: unknown key "server.prot"` (no Go type names).
- [ ] `~`-prefixed paths are rejected with a clear message; Windows paths without a drive
      (`\data`) are documented (resolved against the config dir's volume or rejected – decide, test).
- [ ] `HostEnvironment` takes the lookup function as a parameter (or documents clearly that cmd
      must override it) so M1-10's injected environment is not mixed with the host.
- [ ] Linux defaults are computed per directory so a missing `XDG_CONFIG_HOME` + `HOME` only fails
      the paths that really have no default; error messages name the right variable.
- [ ] Home-relative literals (`AppData`, `Local`, `.config`, `.local`, `share`, `.cache`) are named constants.
- [ ] ADR 0018 gets "Notes (M1-04)": env paths relative to config dir, defaults independent of
      `--config-dir`, Docker sets data/cache/transcode via env, `~` handling.
- [ ] Gates: coverage ≥ 95 % for both packages, golangci-lint 0 issues, go test -race green.

## Notes
- Source: M1-04 reviewer findings (minor 1–5, nits 1–5).

## Deferred (from review, APPROVE) – pick up with the next change to internal/platform/config|paths
- `paths.ConfigDir`: reject drive-relative `C:cfg` for flag and env (`ErrDriveRelative`) or document the exception in ADR 0018.
- ADR 0018: note that strictness comes from the `yaml.Node` walk (not `KnownFields`) and that an empty/null section is allowed.
- Nits: wrap `ErrInvalid` for invalid-YAML / multiple-documents / duplicate-key errors; include the section in the non-scalar-key error; only `%w` a non-nil default error; `otherOS` test helper side effect.
