# SPK wave 2 - test report

Tester: mivo-tester, 2026-10-31, round 1 of wave 2. Tasks SPK-03, SPK-04 (round 2), SPK-05.

| Task | Verdict |
|---|---|
| SPK-03 package metadata (spkmeta) | **PASS** |
| SPK-04 toolkit walking skeleton, round 2 | **PASS** |
| SPK-05 service control script + harness | **FAIL** (one flaky test, see below) |

## Method

The 45 listed files were copied into a clean detached worktree on HEAD fd3295c (scratchpad `wtspk2`);
everything ran there. `packaging/synology/ffmpeg/build/**` was not tested. The worktree is removed
(`git worktree remove --force`, `prune`). Docker Desktop 29.9.2, named volume `spk-toolkit`
(reused, 3 environments deployed), `--cap-add SYS_ADMIN` default.

## SPK-03 - PASS

| Check | Result |
|---|---|
| `go vet ./packaging/...` | clean |
| `go test -race -cover ./packaging/synology/internal/spkmeta` | ok, coverage 100.0 % (threshold 85 %, task 90 %) |
| `golangci-lint run --allow-parallel-runners ./packaging/...` | 0 issues |
| `golangci-lint fmt --diff ./packaging/...`, `go mod tidy -diff` | clean |
| `rg -i -g '!*_test.go' mivo packaging/synology/internal/spkmeta` | only the two import paths in `package.go` (module path, no string literal) |
| Version table | tests cover 0.1.0-alpha.2 -> 0.1.0-1020, rev 1 -> 1021, final -> 9000, 0.0.0-dev -> 0000, errors, monotonic list (`TestMapVersionFollowsOD_SPK_5`, `TestMapVersionRejectsInvalidVersions`, `TestMapVersionIsMonotonic`) |
| Generated files, all families | `build.env` for x86_64/armv8/armv7 (0.1.0-alpha.2) and armv7 final (0.1.0 -> `DSM_VERSION='0.1.0-9000'`, `BETA='no'`) plus `package.env`: 0 lines violate `^[A-Z_][A-Z0-9_]*='[^']*'$`, no CR, the 12 fields in order, armv7 `ARCH='armv7 armada38x monaco'` |
| End-to-end with SPK-04 | each generated `build.env` (ID `mivo`, binary renamed to `prebuilt/bin/mivo`) was fed to the toolkit container: 3 SPKs built, rc 0. INFO: `package="mivo"`, `version="0.1.0-1020"`, `arch` = `x86_64` / `armv8` / `armv7 armada38x monaco`, `adminport="8460"`, `beta="yes"`, `os_min_ver="7.2-64570"`, no `adminurl`. Umlauts in `DESCRIPTION_GER` were accepted by `run.sh` and `INFO.sh`. |

## SPK-04 round 2 - PASS

- `bash packaging/synology/toolkit/test.sh` (exit 0, "all toolkit tests passed"): 59 `ok`, 0 `FAIL`. Covers shellcheck v0.11.0, build-stage
  errors, 3 packs with `--cap-add SYS_ADMIN`, second run with `--network none`, INFO.sh rejection table (11 characters, PORT, DSM_VERSION,
  OS_MIN_VER, family and arch list), "no adminurl", "no beta key for a release", failure runs, tarball pin checks.
- Unpacked SPKs (from the end-to-end run): members `INFO LICENSE conf/ conf/privilege package.tgz scripts/ ...`, no signature member;
  `package.tgz` modes `bin/<id>`, `ffmpeg`, `ffprobe` 0755; INFO as above.
- Hostile `build.env` (on a copy of the project, shared cache untouched):
  - `ID=$(touch /tmp/pwned)` -> `failed: build.env lines other than NAME='literal' assignments or comments: 13:ID=$(touch /tmp/pwned)`, rc 1, nothing sourced.
  - `DISPLAY_NAME='It's'` -> same refusal, rc 1.
- Tampered tarball checksum: tested on a throw-away volume and an image built from a copy of `tarballs.sha256` with the first hash corrupted:
  `failed: SHA-256 of base_env-7.2.txz is ff93a412..., pinned 00000000...; file deleted`, rc 1, no SPK. The shared volume was not modified
  (its 7 tarballs are intact); throw-away volume and image are removed.
- shellcheck v0.11.0: `run.sh`, `test.sh`, `stage-demo.sh`, `SynoBuildConf/{build,install}`, `INFO.sh` with `-s bash`; demo scripts with `-s sh`: all clean.
- Brand: `rg -i mivo packaging/synology/package packaging/synology/toolkit packaging/synology/test` -> no hits.
- Round-2 items: default `--cap-add SYS_ADMIN`, grammar check before sourcing, INFO format checks, install error signalling (exits with `Error:` lines,
  `test.sh` build-stage cases), tarball pins, no `adminurl`, beta only for `BETA=yes`: all verified by the runs above.

## SPK-05 - FAIL

| Criterion | Result |
|---|---|
| shellcheck v0.11.0 `-s sh` on `start-stop-status`, `common.sh`, `package.env`; bash-side files of the harness | met, no findings |
| Brand-free (`rg -i mivo packaging/synology/package`) and fixture uses `DEMO_*` | met (bats cases 2/100/102) |
| prestart/prestop/log exit 0, unknown argument 1 + usage, status 0/3/150, stale PID, graceful stop, SIGKILL, transcode cleanup, log rotation, early exit + log tail, double start, boot start, any-directory call | met: bats 102 tests; dash (`SCRIPT_TEST_SHELLS=dash`) 36 tests all ok |
| One documented command passes on Docker Desktop | **not met reliably**: full run 1: 101 ok, 1 not ok; full run 2: 102 ok |

### Failure: flaky `stop kills server that ignores sigterm`

`packaging/synology/test/scripts/start-stop-status.bats:238`

```
not ok 49 stop kills server that ignores sigterm [bash --posix]
#   `"$2"' failed
# SIGKILL came before the stop timeout
```

The assertion is `((SECONDS - started >= 2))`. Reproduced with `--filter "stop kills"`: 3 failures in 18 case runs (any of the three shells), 1 failure in
the first full run. Cause is the test, not the script: inside the bats container (Docker Desktop/WSL2) the wall clock lags. A control loop of
`sleep 1; sleep 1` showed `/proc/uptime` advancing 2.0 s while both bash `SECONDS` and `date +%s` advanced 0 or 1 (6 of 80 iterations). The script did
sleep the 2 s. Fix: measure with `/proc/uptime` (monotonic) in the test, or drop the lower bound; the graceful-stop `< 5` check has the same exposure.
Do not weaken the assertion without replacing it by a monotonic measurement.

Side note (not a failure): `wait_for_health` in `start-stop-status` builds its 30 s budget from `date +%s`, so a lagging or jumping wall clock stretches or
shortens the budget. Counting poll iterations would be robust; the task does not require it.

## History

- Round 1 (wave 2): SPK-03 PASS, SPK-04 PASS, SPK-05 FAIL.
