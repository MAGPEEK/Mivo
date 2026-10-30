# M1-04 test report

Status: PASS (round 1)
Scope: `./internal/platform/config/...`, `./internal/platform/paths/...`, module hygiene, archtest, govulncheck. `scripts/gen-testmedia` (M1-18) deliberately not part of this run. No web, Docker, media or E2E gates apply to this task.

## Gates

| Gate | Command | Result |
| --- | --- | --- |
| Vet | `go vet ./internal/platform/config/... ./internal/platform/paths/...` | exit 0 |
| Test | `go test -race -count=1 -cover ./internal/platform/config/... ./internal/platform/paths/...` | ok config 98.9 %, ok paths 98.0 % (threshold 85 %) |
| Lint | `golangci-lint run <scope>` | `0 issues.` |
| Format | `golangci-lint fmt --diff <scope>` | exit 0, no diff |
| Mod tidy | `go mod tidy -diff` | exit 0, no diff |
| Mod verify | `go mod verify` | `all modules verified` |
| Archtest | `go test -race -count=1 ./internal/archtest/...` | ok |
| Vulnerabilities | `go tool "-modfile=tools/go.mod" govulncheck ./internal/platform/...` | `No vulnerabilities found.` |
| Layer rule | `go list -deps ./internal/platform/config ./internal/platform/paths` | only `brand`, `paths`, `go.yaml.in/yaml/v3`; no domain/app/adapters edges |

Dependency: `go.mod` gains only `require go.yaml.in/yaml/v3 v3.0.5`; `go.sum` has the two matching hashes (module and go.mod). Tool dependencies stay in `tools/go.mod`.

## Acceptance criteria

| # | Criterion | Verdict | Evidence |
| --- | --- | --- | --- |
| 1 | paths computes config/data/cache/transcode from injected Environment (GOOS, LookupEnv, HomeDir), pure, matches ADR 0018 table, brand names | met | `paths.go:115-169`; `TestDefaults` (7 cases: Windows with/without/empty LOCALAPPDATA, Linux home fallbacks, absolute XDG, relative/empty XDG ignored, no home needed when XDG set), `TestDefaultsWithoutHomeDir`. Windows uses `brand.DisplayName`, Linux `brand.ID`; transcode is `<cache>/transcode` on Linux and `<root>\transcode` on Windows, as in the ADR table. |
| 2 | Config dir: flag > `MIVO_CONFIG_DIR` > OS default, table-tested | met | `TestConfigDir` (flag over env, env over default, default, cleaning), `TestConfigDirErrors` (empty env var, no home). Black-box probe 5 confirmed env-selected dir. |
| 3 | `paths.Ensure` creates dirs 0o750, wrapped error names failing path, tested in TempDir | met | `paths.go:186-198`; `TestEnsureCreatesDirectories` (nested, duplicates, mode check on non-Windows), `TestEnsureErrors` (error contains blocked path, `ErrEmptyPath`). |
| 4 | `config.Load(options)` returns Config with Server{Host,Port}, Log{Level,Format}, Paths{Data,Cache,Transcode}; defaults 8460/info/text | met | `config.go:40-132`; `TestLoadWithoutFileUsesDefaultsAndCreatesNoFile` compares the full struct. Probe 1 output: `Port:8460 Level:info Format:text`. |
| 5 | Precedence default < YAML < env table-tested for every key | met | `TestLoadPrecedence`: 7 keys x 4 layers (default, file, env, env over file) = 28 subtests, all pass. `TestEverySettingHasAFileValue` guards that the file mapping covers all settings. |
| 6 | Missing config.yaml: defaults, no error, no file created | met | Test above asserts `fs.ErrNotExist` after Load. Probe 1: exit 0, 0 files in the dir; probe 1b: a nonexistent config dir stays nonexistent. |
| 7 | Unknown YAML key: error with key name and file path | met | `TestLoadUnknownKeyNamesKeyAndFile` (nested typo, flow mapping `server: {prot: 1}`, unknown section, `paths.ffmpeg`). Probe 2: `parse <dir>\cfg2\config.yaml: yaml: unmarshal errors: line 1: field prot not found in type config.fileServer`, exit 1. |
| 8 | Invalid values name key or env var: port out of range, non-numeric `MIVO_SERVER_PORT`, unknown level/format, empty path | met | `TestLoadInvalidValues` (12 cases, each `errors.Is ErrInvalid`, name key or env var, file cases also name file). Probe 3: `environment variable MIVO_SERVER_PORT: invalid configuration value: port 99999 is outside 0-65535` and `... port "abc" is not a number`. |
| 9 | Relative paths in config.yaml resolve against config dir | met | Precedence cases for the three path keys set `relative: true` and expect `filepath.Join(configDir, value)`. Probe 4: `paths.data: rel-data` became `<cfgdir>\rel-data`. |
| 10 | Env prefix built from `brand.EnvPrefix`, no `MIVO` literal | met | `settings.go:59` and `paths.go:88` use `brand.EnvPrefix`; case-sensitive search for `MIVO` in the non-test and test sources of both packages finds nothing (only lower-case `mivo` in import paths). |
| 11 | Table-driven tests; coverage >= 85 % config and paths | met | 98.9 % and 98.0 % (see above). |
| 12 | Gates: golangci-lint, go test, coverage check | met for scope | Lint 0 issues, tests green with `-race`. Repo-wide `go test ./...` was not run because `scripts/gen-testmedia` is in progress (M1-18), per instruction. |

## Black-box probe

A throwaway `main` was built through `go build -overlay` (virtual file `internal/zzprobe/main.go`, nothing written into the repo; verified `internal/zzprobe` does not exist) and run against temp config dirs in the scratchpad with real `HostEnvironment()`:

- Missing file: defaults, exit 0, no file created.
- Unknown key `server: {prot: 1}`: exit 1, message names `prot` and the full path of `config.yaml`.
- `MIVO_SERVER_PORT=99999` and `=abc`: exit 1, message names `MIVO_SERVER_PORT`.
- YAML `port: 9000`, `level: debug`; then env `MIVO_SERVER_PORT=7777`, `MIVO_LOG_LEVEL=WARN`: result port 7777, level `warn` (env wins, value normalised to lower case); untouched keys keep YAML value / OS default.

## Findings (non-blocking)

- The unknown-key message exposes the internal Go type: `field prot not found in type config.fileServer`. It names key and file as required, but a user-facing message would be cleaner without `config.fileServer`. It also does not show the full key path (`server.prot`). Suggestion only; no criterion is affected.

## History

- Round 1: PASS.
