# M1-04b test report

Status: PASS (round 1)

Scope: `./internal/platform/config/...`, `./internal/platform/paths/...` plus repo-wide `go build`.
Host: Windows 11, go1.27.3, golangci-lint 2.15.0.

## Gates

| Gate | Command | Result |
|---|---|---|
| Build (repo-wide) | `go build ./...` | OK, no output |
| Vet | `go vet <scope>` | clean |
| Vet (Linux) | `GOOS=linux go vet <scope>` | clean |
| Tests + race | `go test -race -count=1 -cover <scope>` | config ok 99.3 %, paths ok 100.0 % |
| Coverage >= 95 % | `go tool cover -func` | total 99.5 %; only `config.go:103 resolveConfigDir` is below 100 % (85.7 %) |
| Lint | `golangci-lint run <scope>` (also with `GOOS=linux`) | 0 issues |
| Format | `golangci-lint fmt --diff <scope>` | no diff |
| Layer rules | `go test -race -count=1 ./internal/archtest/...` | ok |
| Linux tests in Docker | `docker run golang:1.27 ...` | NOT RUN: Docker Desktop is up, but the image is not local and the pull failed with "authentication required - incorrect username or password". Linux code is only covered by `GOOS=linux go vet`, `GOOS=linux golangci-lint` and the OS-independent `Environment{GOOS: "linux"}` unit tests. |

The `go tool -modfile=tools/go.mod` route cannot run golangci-lint: tools/go.mod has no
golangci-lint tool entry, so the winget-installed binary was used.

## Acceptance criteria

Black-box probe: `probe.exe` was built with `go build -overlay` from a file outside the repo
(`cmd/zzprobe/main.go` exists only in the overlay; `git status` shows nothing new in `cmd/`).
It calls `config.Load` with `paths.HostEnvironment(os.LookupEnv)` against temp config dirs in
the scratchpad. Location: `C:\Users\stefa\AppData\Local\Temp\claude\C--Users-stefa-Documents-Mivo\7bf363fe-259b-4e6d-90ab-44ed97076491\scratchpad\probe.exe`.

1. **logging.* constants, `config.LogLevel*`/`LogFormat*` removed: met.**
   `grep -rn "LogLevel\|LogFormat"` finds only the private `keyLogLevel`/`keyLogFormat`.
   Defaults use `logging.LevelInfo`/`logging.FormatText` (config.go:121).
2. **Explicit null is an error with key, file, line: met.**
   - `paths:\n  data:` -> `...\config.yaml: line 2: key paths.data: invalid configuration value: the key has no value (YAML null); set a value or remove the key to use the default`
   - `level: ~` -> `...: line 2: key log.level: invalid configuration value: the key has no value (YAML null) ...`
   - `format: null` behaves the same.
   - Tests: `TestLoadRejectsExplicitNull` (file_test.go:46).
3. **Second YAML document rejected: met.**
   `...\config.yaml: line 3: multiple YAML documents ("---") are not supported; keep a single document`.
   Test: `TestLoadRejectsMultipleDocuments` (file_test.go:80); a leading `---` is accepted (`TestLoadAcceptsLeadingDocumentMarker`).
4. **Relative LOCALAPPDATA ignored, relative home unknown, `Config.Paths` absolute: met.**
   - `LOCALAPPDATA=rel\dir` with a valid USERPROFILE gives `ConfigDir=C:\Users\stefa\AppData\Local\Mivo` (falls back to home).
   - A relative `LOCALAPPDATA` plus a relative `USERPROFILE` gives `ERROR: config directory: default config directory: LOCALAPPDATA and USERPROFILE are unset or not absolute: home directory is unknown`.
   - Relative path values from file and env came out absolute (`...\envcase\envdata`).
   - Tests: `TestLoadPathsAreAlwaysAbsolute` (paths_test.go:14), `TestDefaultsWithoutUsableHome`.
5. **Unknown-key error on one line: met.**
   `...\config.yaml: line 2: unknown key "server.prot"` (also `line 1: unknown key "foo"`). No Go type names.
   Duplicate keys are errors as well: `line 3: duplicate key "server.port"` and `line 3: duplicate key "server"`.
6. **`~` rejected, Windows drive-less paths decided and tested: met.**
   - `data: ~/media` -> `line 2: key paths.data: invalid configuration value: "~/media": path starts with "~", which is not expanded to the home directory; use an absolute path`. Also for `'~'` and for `MIVO_PATHS_DATA=~/x`.
   - `\data` with a config dir on `C:` -> `C:\data`.
   - With a config dir on a different volume (`subst Z:`), `\srv\data` -> `Z:\srv\data` and `/srv/cache` -> `Z:\srv\cache`.
   - `C:data` -> `drive-relative paths such as "C:data" are not supported; use "C:\data"`. `D:\media` is kept.
   - Tests: `TestLoadRejectsHomeShorthand`, `TestLoadWindowsPathsWithoutDrive`, `TestResolve`.
7. **`HostEnvironment` takes the lookup as a parameter: met.**
   Signature `HostEnvironment(lookupEnv func(key string) (string, bool))` (paths.go:99). The only non-test caller is the probe.
   Test: `TestHostEnvironmentUsesTheGivenLookup`.
8. **Linux defaults per directory, error names the right variable: met.**
   Verified through `TestDefaultsWithoutUsableHome` (paths_test.go:166) and the code (`xdgDefaults` computes each directory with its own `baseDir`). The error text names the variable, e.g. `LOCALAPPDATA and USERPROFILE are unset or not absolute`. Not executed on a real Linux host (see Docker note).
9. **Home-relative literals are named constants: met.**
   paths.go:44-49 (`windowsAppDataDirName`, `windowsLocalDirName`, `xdgConfigDirName`, `xdgLocalDirName`, `xdgShareDirName`, `xdgCacheDirName`).
10. **ADR 0018 "Notes (M1-04)": met.**
    docs/adr/0018-configuration-and-os-paths.md:51-90 covers env paths relative to the config dir, defaults independent of `--config-dir`, Docker env, `~`, Windows drive-less paths, strict YAML and the injected environment.
11. **Gates (coverage >= 95 %, lint 0 issues, race green): met.** See the gate table.

Additional probes:
- Missing config.yaml -> defaults (port 8460, info, text, `%LOCALAPPDATA%\Mivo\{data,cache,transcode}`); no file is created.
- Env overrides YAML: file `port: 9000`, `level: debug`, `data: filedata` with `MIVO_SERVER_PORT=9100`, `MIVO_LOG_LEVEL=warn`, `MIVO_PATHS_DATA=envdata` gave Port 9100, Level warn, Data `<configdir>\envdata`. File-only `host: filehost` is kept.
- Env errors name the variable: `environment variable MIVO_PATHS_DATA: ... path must not be empty`, `MIVO_LOG_LEVEL: "loud" is not one of debug, info, warn, error`, `MIVO_SERVER_PORT: port "abc" is not a number`.

## Findings

None blocking.

Observations:
- Linux tests were not executed on Linux (Docker image pull failed on authentication). Re-run the package tests in a `golang:1.27` container once the registry login works.
- `config.go:103 resolveConfigDir` is at 85.7 % (the `filepath.Abs` error branch). This is above the threshold and not a failure.

## History

- Round 1: PASS.

REPORT
status: PASS
task: M1-04b
files: docs/reports/M1-04b-test.md
tests: go build ./... OK; go vet (windows + GOOS=linux) clean; go test -race -count=1 -cover config 99.3 % / paths 100.0 % ok; golangci-lint run 0 issues (also GOOS=linux), fmt --diff empty; archtest ok; black-box probe of all criteria OK; Linux container run not possible (Docker Hub pull: authentication required)
findings:
- none blocking
- Linux package tests not run in Docker (image pull failed, authentication); only the GOOS=linux vet and lint passed
- resolveConfigDir is at 85.7 % coverage, above the threshold
next: hand over to mivo-reviewer; optionally re-run the Linux container test after docker login
