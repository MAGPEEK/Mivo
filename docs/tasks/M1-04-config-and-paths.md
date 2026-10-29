# M1-04 Configuration (YAML + env overrides) and OS-specific paths
Owner: mivo-backend
Depends on: M1-02, M1-03
Files/areas: `internal/platform/config/**`, `internal/platform/paths/**`, `go.mod`, `go.sum`

## Goal
The server reads an optional `config.yaml` from an OS-appropriate config directory, applies
environment overrides and fails fast with a precise message on invalid input. This
implements ADR 0018.

## Acceptance criteria
- [ ] `paths` computes default config, data, cache and transcode directories from an injected
      environment (`GOOS`, env lookup function, home directory). It is pure and testable for
      Windows and Linux on any host. Values match the ADR 0018 table and use brand names.
- [ ] Config directory resolution: explicit flag value > `MIVO_CONFIG_DIR` > OS default.
      Table-tested.
- [ ] `paths.Ensure(...)` creates the directories (mode `0o750`) and returns a wrapped error
      naming the failing path. Tested in `t.TempDir()`.
- [ ] `config.Load(options)` returns a `Config` struct with `Server{Host, Port}`,
      `Log{Level, Format}` and `Paths{Data, Cache, Transcode}`. Defaults come from ADR 0018
      (port 8460, level info, format text).
- [ ] Precedence default < YAML < env is table-tested for every key.
- [ ] A missing `config.yaml` uses defaults, returns no error and creates no file.
- [ ] Unknown YAML key (e.g. `server: {prot: 1}`) → error containing the key name and file
      path.
- [ ] Invalid values → error naming the key or env variable: port outside 0–65535,
      non-numeric `MIVO_SERVER_PORT`, unknown log level or format, empty path.
- [ ] Relative paths in `config.yaml` resolve against the config directory.
- [ ] The env prefix is built from `brand.EnvPrefix`. No `MIVO` string literal in these
      packages.
- [ ] Tests: table-driven tests for the above. `go test -cover ./internal/platform/...`
      shows ≥ 85 % for `config` and `paths`.
- [ ] Gates: `golangci-lint run`, `go test ./...` and the coverage check from M1-03 pass.

## Notes
- YAML library: `go.yaml.in/yaml/v3` with `KnownFields(true)` on the decoder. No viper or
  koanf (ADR 0018).
- Keep the env mapping explicit: a small table of key path → setter/parser is clearer than
  reflection and stays under the complexity limits.
- `filepath.Join` uses the host separator. When testing Windows expectations on Linux (or the
  reverse), build expected values the same way, or compare after `filepath.ToSlash`.
- No globals. `Load` takes everything through an options struct (≤ 4 parameters rule).
- Do not add an ffmpeg path setting yet. Milestone 2 adds it with its own validation.
