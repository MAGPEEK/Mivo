# SPK-14 test report (QA gate)

Result: PASS (round 2). Worktree `.claude/worktrees/agent-a1fc14bc2eb406923`, uncommitted against 1fd4dd9. No code edited.

## Gates

| Gate | Result | Evidence |
|---|---|---|
| `go vet ./...` | PASS | no output |
| `golangci-lint run` | PASS | `0 issues.` |
| `golangci-lint fmt --diff` | PASS | no diff |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | PASS | no diff |
| `go test -race -coverprofile` in golang:1.27.3 (Docker, Linux) | PASS | all packages ok; `spkstage` 89.3 % (incl. `main` 0 %), spkicon 96.8 %, spkmeta 100 % |
| go-test-coverage (`.testcoverage.yml`) | PASS | `Total test coverage: 95.9 % (6455/6732)`, threshold 70 % satisfied; domain packages 100 % |
| Linux-only `-force` tests | PASS, not skipped | `TestCheckReplaceableFollowsLinks`, `TestForceNeverReplacesTheRepository`, `TestForceNeverReplacesForeignDirectories`, `TestForceReplacesAStagedProject` ran on Linux (92 test passes, no SKIP) |
| shellcheck `-s sh` on the 8 package scripts | PASS | exit 0 |
| shellcheck on bats files, helpers, fake-server, fixtures | PASS | exit 0 |
| bats suite (`bats/bats:1.14.0` test image) | PASS | `1..295`, 295 ok, 0 not ok, exit 0 |
| `packaging/synology/toolkit/test.sh` (x86_64, armv8, armv7, lint) | PASS | `all toolkit tests passed`, 67 ok, exit 0; includes the new CRLF cases (scripts, conf, wizard `.sh`, port file) and the build.env twelve-field/duplicate/missing cases |
| Layer rule (domain imports) | not applicable | no change under `internal/domain`; `go list -deps` of spkstage imports only stdlib and repo packages |

The toolkit test used a throw-away volume `qa-spk14-toolkit` seeded read-only from `spk-toolkit` and the image tag `spk-toolkit:qa-spk14`. `spk-toolkit` was not modified. The volume, both image tags (`spk-toolkit:qa-spk14`, `spk-script-test:qa-spk14`) and the scratch files have been removed.

## End-to-end check (x86_64)

- Server: `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build` (without `webui`), 14,049,440 bytes.
- FFmpeg: `fetch.sh x86_64` (jellyfin-ffmpeg 8.1.3-1, SHA-256 ok, interim).
- `spkstage -family x86_64 -version 0.1.0-alpha.2 ...` gave exit 0, `staged the x86_64 project (31 files, DSM version 0.1.0-1020)`. Three warnings on stderr: interim build, `ffmpeg` and `ffprobe` dynamically linked.
- `run.sh x86_64` gave exit 0 and `wrote mivo-x86_64-0.1.0-1020.spk`.
- SPK top-level members: INFO, LICENSE, PACKAGE_ICON.PNG, PACKAGE_ICON_256.PNG, `WIZARD_UIFILES/{install_uifile,install_uifile_ger,uninstall_uifile,uninstall_uifile_ger}`, `conf/{privilege,resource}`, `package.tgz`, `scripts/*` (7 scripts, `common.sh`, `package.env`). No signature file and no `.spkstage` anywhere in the SPK.
- `package.tgz` (xz):
  ```
  drwxr-xr-x bin/
  -rwxr-xr-x bin/ffmpeg            109525528
  -rwxr-xr-x bin/mivo               14049440
  -rwxr-xr-x bin/ffprobe           109230488
  drwxr-xr-x port_conf/
  -rw-r--r-- port_conf/mivo.sc            84
  drwxr-xr-x share/licenses/ffmpeg/
  -rw-r--r-- share/licenses/ffmpeg/COPYING.GPLv3
  -rw-r--r-- share/licenses/ffmpeg/SOURCE.txt
  -rw-r--r-- share/licenses/ffmpeg/LICENSE.md
  ```
  `port_conf/mivo.sc` content: `[mivo] title="Mivo" desc="Mivo media server" port_forward="no" dst.ports="8460/tcp"`. `conf/resource` has `"port-config": {"protocol-file": "port_conf/mivo.sc"}`.
- SPK script modes: scripts 0755, `conf/*`, `WIZARD_UIFILES/*`, INFO, LICENSE and icons 0644. `scripts/package.env` is 0755 in the SPK (staged 0644; the toolkit sets modes on `scripts/*`). This is harmless and not a criterion failure.
- INFO (no `adminurl`, `grep -c adminurl` = 0):
  ```
  package="mivo" version="0.1.0-1020" maintainer="MAGPEEK" maintainer_url="https://github.com/MAGPEEK/Mivo"
  arch="x86_64" adminprotocol="http" adminport="8460" os_min_ver="7.2-64570" beta="yes"
  displayname="Mivo" support_move="no" extractsize="227432" toolkit_version="72723" create_time=... (plus description, description_ger)
  ```
- Modes of the staged tree, checked by staging inside Linux (the Windows host shows no modes): directories 0755; `INFO.sh`, `SynoBuildConf/{build,install}`, `scripts/*` except `package.env`, and the three prebuilt executables are 0755; everything else is 0644. Every entry has mtime 1700000000, equal to `-source-date-epoch`.

## `-force` safety (temp dir, built spkstage)

| Case | Result |
|---|---|
| `-out` contains `-binary` (with marker) | exit 1, `-out ... overlaps -binary ...: refusing to use this directory`; `.spkstage`, `bin/server` and `keep.txt` all still present |
| `-out` contains `-binary` (no marker) | exit 1, same refusal; file kept |
| Directory without `.spkstage`/`build.env` | exit 1, `holds no .spkstage and build.env, so it is not a staged project`; `keep.txt` kept |
| `-out` = repo, `repo/packaging`, `repo/packaging/synology/package` | each exit 1, `overlaps -repo ...: refusing to use this directory`; `git status` unchanged, template files still present |
| Rerun on staged project without `-force` | refused (`project directory exists and is not empty (pass -force to replace it)`) |
| Rerun with `-force` | exit 0; the tree listing (names, modes, mtimes) hash is identical to the first run |
| Leftover `.spkstage-*` temp directories | none |

## Acceptance criteria

1. Command and layout: met. The 31-file project has `build.env`, `INFO.sh`, `SynoBuildConf/{depends,build,install}`, 7 scripts plus `common.sh` and a rendered `package.env`, `conf/{privilege,resource}`, `WIZARD_UIFILES/`, `LICENSE`, the two icons, `prebuilt/bin/{mivo,ffmpeg,ffprobe}`, `prebuilt/share/licenses/ffmpeg/*` and `prebuilt/port_conf/mivo.sc`. `SynoBuildConf/install` and `build` coordinate the `port_conf` path, and the SPK contains `port_conf/mivo.sc` in `package.tgz`. The golden-list test also passes.
2. No `adminurl` in the packed INFO: met (see INFO above).
3. Modes and mtimes: met (Linux staging listing, and the golden test).
4. Refusals: met. Covered by `TestStageRefusesBrokenInputs`, `...NonExecutableInputs`, `...BrokenTemplates`, `TestParseOptionsRefusals`, `TestFamilyELFRejectsUnknownFamily` and `TestInterimFFmpegIsStagedWithAWarning`, which all pass. Checked live: the existing-project refusal, and the interim warning on stderr.
5. Go tests: met. They cover the golden list per family, the armv7 `ARCH`, JSON parsing, `DSM_VERSION`/`BETA`, no unreplaced token, and `TestStagingIsReproducible`. `main` is thin (`run(args, stdout, stderr) error`). Coverage of the package is 89.3 % including `main` (0 %), so it is above 80 % excluding `main`. The brand-free test and the spkmeta contract tests (`TestScriptFixtureIsSpkmetaOutput`, `TestCommonShLoadsTheStagedPackageEnv`) pass.
6. Toolkit check: met (real binary and interim FFmpeg to one SPK; tar listing and INFO above).
7. Gates: met (table above).
8. Orchestrator note: met. `package.env` is written from `spkmeta` and the `@TOKEN@` template is deleted. `run.sh` checks CRLF in `scripts/*`, `conf/*`, the port file and wizard `.sh` files, and accepts only the 12 known build.env fields. test.sh has tests for both.

## History

- Round 2: all gates and the end-to-end check PASS (this report).
