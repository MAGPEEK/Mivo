# SPK-13 prerequisites – test report

Scope: WIP commit `aa32125` on top of main plus the uncommitted inspect changes
(`packaging/synology/test/inspect/*`, `toolkit/README.md`). Worktree
`.claude/worktrees/agent-a35f37c6fa854ba02`. Windows 11, Docker Desktop 29.9.2, Git Bash.

Result: **PASS**

## Gates

| Gate | Command (abridged) | Result |
|---|---|---|
| Script bats (BusyBox sh, dash, bash --posix, bash) | `docker run spk-script-test:dev packaging/synology/test/scripts` | 335 cases, 0 not ok, 0 skipped |
| Build-script bats, with pwsh and git (build.sh + build.ps1 parity) | `spk-build-test:dev packaging/synology/test/build` | 86 cases, 0 not ok, 2 skipped ("git is installed; the image without git runs this case") |
| Build-script bats in the image without git/pwsh | `spk-script-test:dev packaging/synology/test/build` | 56 cases, 0 not ok, 3 skipped (pwsh cases); the two git-less cases ran |
| stage.bats, package-gates.bats, fetch.bats | part of the two runs above | all ok |
| Inspect bats | `spk-inspect:dev bats packaging/synology/test/inspect` | 53 cases, 0 not ok |
| shellcheck v0.11.0 | package scripts (`-s sh`), script tests, build.sh, stage.sh, build tests, fake-docker, fake-spkstage, fetch.sh (`-s sh`), package.sh (`-x -s sh`), inspect-spk.sh (`-s sh`), inspect bats and fixture | 7 invocations, exit 0 each |
| `go vet ./packaging/...` | | clean |
| `go test -race -coverprofile ./packaging/...` | | ok; spkstage 88.9 %, recipecheck no statements, spkicon 96.8 %, spkmeta 99.5 %; total 92.6 % (threshold 70 %) |
| `golangci-lint run ./packaging/...` | | 0 issues |
| Repo-wide `go test ./...` | | all ok |
| Repo-wide `golangci-lint run` | | 0 issues |

Web, E2E and Docker image gates do not apply (the scope touches packaging only). Domain layer
untouched (no change under `internal/`; the domain import rule was not re-checked).

## Full test build

`bash packaging/synology/build.sh --test 0.1.0-alpha.2`: rc 0, "done in 48s (test build)", three
SPKs plus `SHA256SUMS`, FFmpeg source archive and `NOT-FOR-RELEASE.txt` in `dist/`.

`inspect-spk.sh` (test mode and `--release`), all six runs: rc 0,
`88 checks, 0 failed, 2 warnings`. The two warnings are the known ones in every run:
`bin/ffmpeg` and `bin/ffprobe` have no GNU ABI note, so the kernel floor is not checked
(SPK-11 smoke test covers it). The new DSM main-menu entry checks all pass: INFO
`dsmuidir="ui"`, `ui/config` parses with one `.url` entry (type url, title Mivo, protocol http,
port 8460, url `/`, allUsers true, icon `images/mivo_{0}.png`), and `ui/images/mivo_<n>.png` are
PNGs of n x n for n = 16, 24, 32, 48, 64, 72, 256.

## SPK-11 smoke test (corpus generated with `go run ./scripts/gen-testmedia`: 45 generated, 0 failed)

| Family | Result |
|---|---|
| x86_64 | `SUMMARY x86_64: 28 passed, 0 failed` |
| armv8 | `SUMMARY armv8: 28 passed, 0 failed` |
| armv7 | `SUMMARY armv7: 82 passed, 0 failed` (three CPU models) |

## Backlog rows, acceptance items

All met (evidence: the suites above plus a read of the diff).

`| spk scripts |` (SPK-06 review)
- `pkg_is_package_directory` replaced by `pkg_package_directory_kind` (shapes
  `/var/packages/<id>/{var,home}`, `*/@appdata/<id>`, `*/@apphome/<id>`; refuses `.`, `..`, `//`)
  and `pkg_resolve_package_directory` (`cd -P`, resolved path checked again, deletion below the
  resolved path); `postuninst` uses it and checks both dirs before deleting. Covered in
  `lifecycle.bats` under all four shells.
- `mkdir` lock (`take_lock`/`release_lock`, stale-holder and wait-timeout breaking,
  `PKG_TEST_LOCK_WAIT_SECONDS`) in `start-stop-status` for start, stop and status; tests incl.
  "lock held too long".
- postinst/postuninst messages go through `pkg_localized`.
- Non-BMP share-name fixtures (`share-names.bash`, 16/17 astral characters) and
  `pkg_name_length` counting UTF-16 units; wizard regex and `pkg_check_share_name` agree.
- Test image pinned: `bats/bats:1.14.0@sha256:5322b877...` and `apk add dash=0.5.13.1-r2
  jq=1.8.2-r0 nodejs=24.22.1-r1`.

`| spk build scripts |` (SPK-07 r2)
- `build.ps1` `#Requires -Version 7.3`; `dest=` always quoted in both scripts.
- Failing git in release mode dies; empty revision refused (build bats, both scripts).
- Dockerfile `stage` heredoc moved to `packaging/synology/stage.sh`, tested by `stage.bats`
  against `fake-spkstage.bash`; fake-docker honours `RELEASE=true` (`FAKE_STAGE_TEST_MODE`).
- Nits: `FAKE_ID`, revision-mismatch and "toolkit did not write" cases, option values starting
  with `-` rejected, message asserted for the empty value.

`| spk/ffmpeg build |` (SPK-16 r2)
- `write_source_notice` computes every value into a variable with `|| fail`, checks 64-hex
  SHAs, exactly one configuration line and one `.tar.sha256` (`package-gates.bats`).
- `-clike '*nonfree*'` in `build.ps1`; `package-gates.bats` named in test/README (comment and
  shellcheck line); recipecheck duplicate check after the configure-name translation;
  `host_tools` returns early when `HOST_PACKAGES` is empty.

`| spk |` wave-2 nits
- SPK-03 AC reworded (brandfree_test); `pkg_dump_info || info_fail` in `INFO.sh`; untracked
  files detected in the toolkit clone (`--untracked-files=all` in `run.sh`); `packageIDPattern`
  `^[a-z0-9]+([.-][a-z0-9]+)*$` (and the matching `common.sh` ID check); regexps compiled once
  (package-level `regexp.MustCompile`).

DSM main-menu entry and inspect extension: `spkmeta/desktopapp.go` with tests (spkmeta 99.5 %
coverage), `dsmuidir="ui"` and `ui/` in the three built SPKs, inspect-spk.sh checks above
pass, 88 checks per SPK.

## Notes (not failures)

- The BACKLOG rows are still present in `docs/tasks/BACKLOG.md`; the orchestrator should
  remove them on commit.
- `toolkit/README.md` still says "53 cases" for the inspect bats (matches the 53 run) and
  "69 checks" only in the historical SPK-07 results paragraph; the current count is 88.
- Test artefacts (`dist/`, `testdata/media`) are git-ignored; `git status` shows only the
  five intended uncommitted files.

## History

- Round 1: PASS.

REPORT
status: PASS
task: SPK-13-pre
files: docs/reports/SPK-13-pre-scripts-test.md
tests: script bats 335/335, build bats 86/86 (+56 in the git-less image), inspect bats 53/53, shellcheck clean, go vet/test -race (packaging 92.6 %), golangci-lint (packaging and repo) 0 issues, go test ./... ok, build.sh --test ok, inspect test and --release 88 checks 0 failed 2 warnings on all three SPKs, SPK-11 smoke 28/28/82 passed
findings: none (BACKLOG rows still to be removed by the orchestrator)
next: orchestrator may proceed to review/commit; remove the four met BACKLOG rows
