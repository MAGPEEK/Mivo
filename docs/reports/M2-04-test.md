# M2-04 test report: configuration of libraries, FFmpeg tools and transcode limit

Status: **PASS** (round 1, 2026-10-30)
Scope: `internal/platform/config/**` (5 new files: `libraries.go`, `library_roots.go`,
`libraries_test.go`, `libraries_internal_test.go`, `tools_test.go`; 7 changed), `internal/platform/paths/**`,
ADR 0018 Notes. Wave 1 runs in parallel; `cmd/mivo/serve.go` carries an unrelated change (restapi
options) that belongs to another task and is not judged here. It compiled and its tests passed.

## Gates

| Gate | Command | Result |
|---|---|---|
| vet (Windows) | `go vet ./internal/platform/... ./cmd/...` | exit 0, no output |
| test + race (Windows) | `go test -race -count=1 -coverprofile=... ./internal/platform/... ./cmd/...` | all `ok`; config 99.7 %, paths 100.0 %, logging 100.0 %, buildinfo 96.9 %, cmd/mivo 88.9 % |
| vet + test + race (Linux) | `docker run golang:1.27.3`, repo mounted `:ro`, `GOCACHE`/`GOPATH` in `/tmp` | vet exit 0; config 99.7 %, paths 97.6 % (Windows-only branches), all `ok` |
| coverage total | `go tool cover -func` | total 97.1 %; the only function below 100 % in the two packages is `config.go:154 resolveConfigDir` 85.7 % (the `filepath.Abs` error branch) |
| coverage gate | `go tool '-modfile=tools/go.mod' go-test-coverage '--config=.testcoverage.yml' '--profile=<profile>'` | `Total coverage threshold (70%) satisfied: PASS`, 97.1 % (646/665) |
| lint | `golangci-lint run ./internal/platform/... ./cmd/...` | `0 issues.` |
| format | `golangci-lint fmt --diff ./internal/platform/... ./cmd/...` | no diff |
| layer rule | `go test -count=1 ./internal/archtest/...` | ok |
| layer rule (independent) | `go list -deps ./internal/domain/...` filtered for `database/sql`, `net/http`, `os/exec`, `internal/adapters`, `internal/app` | no match |
| platform imports | `go list -f '{{join .Imports "\n"}}' ./internal/platform/config ./internal/platform/paths` | stdlib, `brand`, `logging`, `paths`, `go.yaml.in/yaml/v3`. No `internal/domain` (the Notes require this) |
| TODO | grep `TODO` in `internal/platform` | none |

Web, E2E, media corpus and Docker gates: not applicable (platform-only task). Domain coverage is
unchanged, because no domain code was touched.

Note on the coverage gate output: the package line prints the global default `(0%)`, as in the
M2-03 report. The profile total of 97.1 % is above the 70 % threshold, and the two packages are
above the 95 % that the task asks for in any case.

## Acceptance criteria

1. **New keys, each with YAML key, env, default and validation in the code table and in the REPORT: met.**
   The code table is `settings.go:19-30` (`keyToolsFFmpeg`, `keyToolsFFprobe`,
   `keyTranscodeMaxSessions`), `libraries.go:15` (`keyLibraries`) and `config.go:27-35` (defaults).
   ADR 0018 "Notes (M2-04)" has the table. Summary:

   | YAML key | Environment | Default | Validation (observed in the probe) |
   |---|---|---|---|
   | `libraries` (`{name, path, kind}`) | `MIVO_LIBRARIES` (`filepath.SplitList`, replaces the list, name = last element) | empty list | see criteria 2 to 4 |
   | `libraries[].kind` | n/a | `movies` | `movies`, `shows` (case-insensitive); `music` rejected |
   | `tools.ffmpeg` | `MIVO_TOOLS_FFMPEG` | `ffmpeg` | empty/blank rejected; bare name kept; path resolved |
   | `tools.ffprobe` | `MIVO_TOOLS_FFPROBE` | `ffprobe` | as `tools.ffmpeg` |
   | `transcode.max_sessions` | `MIVO_TRANSCODE_MAX_SESSIONS` | `2` | integer 1-8 (orchestrator note); 0, 9, -1, `two`, `2.5`, empty rejected |

   Default < YAML < env is covered by the table in `config_test.go:212-227` (tools, max_sessions)
   and by `TestLoadLibrariesPrecedence` (`libraries_test.go:68`).
2. **Library paths resolve like other paths, errors name key, index and file: met.**
   Probe: relative `../media` starts, relative `media` resolves to `<cfg>\media` and is then rejected as
   inside the config dir, `~/media` rejected, `C:media` rejected (drive-relative). Example:
   `config.yaml: line 2: key libraries[0].path: invalid configuration value: "C:media": drive-relative paths such as "C:data" are not supported; use "C:\data"`.
   From the environment: `environment variable MIVO_LIBRARIES: libraries[0].path: ...`.
3. **Duplicate, nested, config/data/cache/transcode overlap; case-insensitive on Windows: met.**
   Probe (see table below): all rejected, each with a one-line error that names file, line, key
   `libraries[i].path` and the counterpart (`libraries[0]` or the named directory).
   `TestLoadComparesLibraryRootsByOS` (`libraries_test.go:385`) injects `GOOS` (windows/linux) and
   passes on both hosts: Windows folds case, Linux does not (negative cases).
   `TestWithin` (`paths_test.go:424`) has 16 cases including `C:\Media` vs `c:\MEDIA\Movies` and `\\NAS\Share`.
4. **Non-existent root accepted: met.** Probe: `C:\Media` and `C:\Mediathek` are nonexistent
   and start without error (and were not created). Test: `TestLoadAcceptsMissingLibraryRoot`.
5. **`tools.*`: bare name kept, path resolved, empty rejected: met.** Probe: `ffmpeg-custom` starts,
   `C:\tools\ffmpeg.exe` and `bin\ffmpeg.exe` start, `''`, `'   '`, `null` and `MIVO_TOOLS_FFMPEG=` are
   rejected, `~/ffmpeg` and `C:ffmpeg.exe` are rejected. Unit: `TestLoadToolValues` (`tools_test.go:15`).
6. **Unknown keys inside a library entry: met.** Probe:
   `config.yaml: line 3: unknown key "libraries[1].pth"` (exact format from the task). Unit:
   `file_test.go:12-45` incl. flow style and the merge key.
7. **BACKLOG row "config/paths": met.**
   - Drive-relative `C:cfg` config dir: probe `--config-dir C:cfg`, `MIVO_CONFIG_DIR=C:cfg` and
     `MIVO_CONFIG_DIR=D:` all exit 1 with `config directory: flag value|environment variable MIVO_CONFIG_DIR: "C:cfg": drive-relative paths ...`.
     `paths.ErrDriveRelative` is asserted in `paths_test.go:348-360` and `config_test.go:362`. The check follows the
     injected GOOS (`paths.go:293-305`), and `C:cfg` is kept as an ordinary name for GOOS linux.
   - ADR 0018 Notes: the `yaml.Node` walk and the empty-section note are present (`0018...md`, first added bullet).
     The diff only adds lines after the existing Notes; nothing existing changed.
   - `ErrInvalid` wrapping: invalid YAML (`file.go:102`, `125`), multiple documents (`file.go:128`), duplicate
     key (`file.go:204`, `296`) now `errors.Is(err, ErrInvalid)`; asserted in `file_test.go:91-190`.
     The non-scalar key stays `ErrUnknownKey` and now names the section: `line 2: unknown key in section server: keys must be plain names`
     and `line 1: unknown key at the top level: ...` (probe 80 and 81).
   - `%w` only for a non-nil default error: `config.go:194-197` introduces `errNoDefault`;
     `TestRequirePathsWithoutDefaultReason` covers it.
   - `otherOS` helper: no longer writes into `f.vars`; it builds a layered `Environment` (`paths_test.go:126-145`).
8. **Tests: table tests for every key, list parsing, overlap rules, Windows case-insensitivity; both packages >= 95 %: met.**
   config 99.7 %, paths 100.0 % (Windows) and 97.6 % (Linux).
9. **Gates: lint, layer rules, race tests, coverage gate: met**, see the gate table.
10. **Orchestrator note: met.** Max 8 (probe: 8 starts, 9 fails with `is outside 1-8; the server runs at most 8 FFmpeg sessions at once`).
    No `MIVO_LIBRARIES=/media` preset exists in the repo (`internal/platform` and `cmd` contain none). Nothing is wired in `cmd`
    for this task (`git diff` of `cmd/mivo` shows only the unrelated restapi change).
11. **Code rules: met.** Longest new function is `Load` (40 lines), at most 4 parameters, lint (including complexity) clean.
    `platform` does not import `domain`.

## Black-box probe with the real binary

I built `cmd/mivo` into the scratchpad (`go build`, the same package as `go run ./cmd/mivo`) and ran 83
cases. Each case had its own `MIVO_CONFIG_DIR`, `MIVO_PATHS_DATA|CACHE|TRANSCODE` under the scratchpad,
`MIVO_SERVER_HOST=127.0.0.1`, `MIVO_SERVER_PORT=0`, and a config.yaml written for that case. "starts" means the process
was still running after 4 s and I killed it. Every failure is `exit 1`, one line, starting with
`mivo: load configuration: <file or variable>: ...`.

| Case | Result |
|---|---|
| valid list (2 libraries, kind `movies`/`SHOWS`, with and without name), `libraries: []`, no key, empty sections | starts |
| duplicate roots; `a` and `a\.\` | `line 3: key libraries[1].path: ... is the same directory as the root ... of libraries[0]` |
| `a` then `a\b`; `a\b` then `a` | `lies inside the root ...` / `contains the root ...` of `libraries[0]` |
| `a` and `ab` (shared prefix) | starts (no false positive) |
| root equal to data / cache / transcode / config dir | `is the same directory as the data directory (paths.data) "..."`, similarly cache, transcode, `the config directory` |
| root inside data / transcode / config dir; root containing the base of all four | `lies inside the ...`; `contains the config directory` |
| `C:\Media` + `c:\media\Movies`; `C:\Media` + `c:\MEDIA`; `C:/Media` + `C:\Media\x` (YAML) | rejected: `lies inside` / `is the same directory` |
| `MIVO_LIBRARIES=C:\Media;c:\media\Movies` | `environment variable MIVO_LIBRARIES: libraries[1].path: ... lies inside the root "C:\\Media" of libraries[0]` |
| `C:\Media` + `C:\Mediathek` | starts |
| unknown field `pth` / `colour` | `line 3: unknown key "libraries[1].pth"` / `"libraries[0].colour"` |
| `kind: music`; `kind:` (null) | `key libraries[0].kind: ... "music" is not one of movies, shows`; null error |
| missing `path`; blank `name`; duplicate field in an entry | `key libraries[0]: ... the entry has no "path"`; `key libraries[0].name: ...`; `key libraries[0].path: ... duplicate key` |
| `libraries:` null; mapping; scalar entry; duplicate top-level key | clear errors with line and key |
| `MIVO_LIBRARIES=<base>\m1;<base>\m2` (Windows `;`) | starts |
| same value twice; nested; `~\m1`; `C:m1`; equal to data dir | rejected with `environment variable MIVO_LIBRARIES: libraries[N].path: ...` |
| `a;;b` (empty element) | `libraries[1].path: ... path must not be empty` |
| `MIVO_LIBRARIES=` (empty) with an invalid list in the file | starts (an empty list replaces the file list, as documented) |
| `MIVO_LIBRARIES=<path>` with a duplicate list in the file | starts (the file list is replaced, not merged) |
| `max_sessions` 0, 9, -1, `two`, `2.5`, `''`, null (file); 0, 9, `two` (env) | all exit 1 naming `transcode.max_sessions` (file, with line) or `MIVO_TRANSCODE_MAX_SESSIONS` (env) |
| `max_sessions` 1 and 8 (file); 8 (env) | starts |
| `tools.ffmpeg: ''`, `tools.ffprobe: '   '`, null, `MIVO_TOOLS_FFMPEG=`, `MIVO_TOOLS_FFPROBE='  '` | `... program must not be empty; use a name looked up on PATH or a path` |
| `tools.ffmpeg` bare name, absolute path, relative path | starts |
| `~/ffmpeg`, `C:ffmpeg.exe`, unknown `tools.ffplay` | rejected with key and line |
| `--config-dir C:cfg`, `MIVO_CONFIG_DIR=C:cfg`, `MIVO_CONFIG_DIR=D:` | `ErrDriveRelative` message, exit 1 |
| invalid YAML; two documents; duplicate key; non-scalar key in a section / at the top level; unknown top-level key `libary` | `invalid configuration value: invalid YAML: ...`; `line 3: ... multiple YAML documents`; `line 3: key server.port: ... duplicate key`; `line 2: unknown key in section server: ...`; `line 1: unknown key at the top level: ...`; `line 1: unknown key "libary"` |

Side effects: before and after the run `%LOCALAPPDATA%\Mivo`, `%APPDATA%\Mivo`, `%ProgramData%\Mivo`,
`C:\Media`, `C:\Mediathek`, `C:\media` and `%USERPROFILE%\Mivo` do not exist. The server created only
`data/`, `cache/`, `transcode/` and `server.db*` inside the per-case scratchpad directories, which I removed afterwards.
`C:\tools` exists, is empty and dates from 2026-10-19, so it was there before this run (not created by me, not touched). No `mivo`
process was left running. No file in the repository was created or modified by the tests (no `coverage.out`).

## Findings (non-blocking)

- F1 (`libraries.go:178`, Windows only): `MIVO_LIBRARIES=/media/a:/media/b` on Windows is one element because `:` is
  not the Windows list separator. `paths.Resolve` turns it into the single root `C:\media\a:\media\b` and the server
  starts without a message. It follows the task ("OS path list via `filepath.SplitList`") and ADR 0018 Notes say `;` on
  Windows, but the user who copies a Linux value gets a silent odd root. A check for `:` after the volume name, or a hint in the
  docs for the Docker preset (M2-35), would help. M2-32 or the walker will see it at the latest when the root does not exist.
- F2 (`library_roots.go`, `paths.Within`): the comparison is lexical. `\\?\C:\Media` vs `C:\Media\x` and symlinks/junctions
  are not detected. This is the known BACKLOG row "config/paths" (line 28, from the M2-04 review), so not new.
- F3: the library errors repeat the long absolute path twice with `%q`, so Windows backslashes are doubled and the line gets long
  (probe 4: roughly 450 characters with deep temp dirs). It stays one line and names key, line and file. Cosmetic.
- F4: a trailing separator in `MIVO_LIBRARIES` (`a;`) yields an empty element and the error `libraries[1].path: path must not be empty`.
  This is correct but may surprise. The error already names the index.

## History

- Round 1 (2026-10-30): all gates green, all criteria met, PASS.

REPORT
status: PASS
task: M2-04
files: docs/reports/M2-04-test.md
tests: go vet, go test -race -count=1 (internal/platform/..., cmd/...) on Windows and Linux (golang:1.27.3, repo :ro) ok; config 99.7 %, paths 100.0 % / 97.6 %, total 97.1 %; go-test-coverage gate satisfied; golangci-lint run 0 issues; fmt --diff clean; archtest ok; 83-case black-box probe with the real binary behaves as documented
findings:
- All criteria and the taken-over BACKLOG items (ErrDriveRelative for flag/env, ErrInvalid wrapping, section in the non-scalar-key error, errNoDefault, otherOS helper, ADR 0018 Notes) are met; max_sessions 1-8 confirmed, no Docker preset in this task.
- F1 (non-blocking, Windows): `MIVO_LIBRARIES=/a:/b` becomes one odd root `C:\a:\b` without an error, because `:` is no list separator there.
- F2 (non-blocking, known BACKLOG row): `paths.Within` is lexical; `\\?\` prefixes and symlinks/junctions are not compared.
- F3/F4 (cosmetic): long, doubled-backslash paths in overlap errors; trailing `;` gives "path must not be empty" for the last index.
- No real directories were created (checked `%LOCALAPPDATA%\Mivo` etc.); only scratchpad probe dirs, cleaned up.
next: Forward to mivo-reviewer; no implementer rework needed.
