# SPK-13-pre round 2 - test report

Scope: uncommitted changes on top of main 5a518fa in worktree
`.claude/worktrees/agent-ac721c6e59a78d284` (BACKLOG rows `spk scripts`, `spk build scripts`,
`spk inspect`, all "SPK-13-pre review minors"). Windows 11, Docker Desktop, Git Bash.

Result: **PASS**

## Gates

| Gate | Result |
|---|---|
| Script bats (BusyBox sh, dash, bash --posix, bash), `spk-script-test:dev test/scripts` | 351 ok, 0 not ok |
| Build-script bats incl. build.ps1 parity, `spk-build-test:dev test/build` (also stage.bats, package-gates.bats, fetch.bats) | 90 ok, 0 not ok (2 skips: "git is installed; the image without git runs this case") |
| Same suite in the git-less/pwsh-less image `spk-script-test:dev test/build` | 59 ok, 0 not ok (4 skips, pwsh cases) |
| Inspect bats `spk-inspect:dev bats test/inspect` | 57 ok, 0 not ok |
| shellcheck v0.11.0 (7 invocations per test/README plus inspect-spk.sh `-s sh`, inspect bats and fixture) | exit 0 each |
| `go vet ./packaging/...` | clean |
| `go test -race -coverprofile ./packaging/...` | ok; spkstage 88.9 %, spkicon 96.8 %, spkmeta 99.5 %, total 92.7 % (>= 70 %) |
| `golangci-lint run ./packaging/...` | 0 issues |
| `go test ./...` (repo-wide, once) | all ok |
| `build.sh --test 0.1.0-alpha.2` | rc 0, "done in 65s (test build)", three SPKs, SHA256SUMS, source tar, NOT-FOR-RELEASE.txt |
| `inspect-spk.sh` test and `--release`, x86_64, armv8, armv7 (6 runs) | each rc 0, `91 checks, 0 failed, 2 warnings`; the warnings are the known `bin/ffmpeg` / `bin/ffprobe` "no GNU ABI note" |
| SPK-11 smoke (corpus generated: 45 generated, 0 failed) | x86_64 `28 passed, 0 failed`; armv8 `28 passed, 0 failed`; armv7 `82 passed, 0 failed` |

Not applicable: web, E2E, Docker server image (packaging only; nothing under `internal/` changed,
so the domain import rule was not re-checked).

## Backlog rows

All items met (suites above plus a read of the diff).

`| spk scripts |`
- Action lock no longer breaks a live holder after 10 s: after the wait it breaks only when the
  holder is not a `start-stop-status` process (`is_action_process`), at once for an earlier boot,
  dead holder or own PID; waiter logs the action it waits for once. Bats: "status broke the lock of
  a live action" (>= 2 s wait), earlier-boot case (< 3 s), "breaking the lock touched its holder".
- mkdir-fail/`-d` race: one retry in `take_lock` (`lock_create_retries`); bats uses a fake mkdir
  ("the fake mkdir was not used" guard).
- `pkg_resolve_package_directory` / `pkg_package_directory_kind`: prefix limited to
  `/volume<N>/@appdata|@apphome/<id>` with one component (`pkg_volume_directory_kind`); lifecycle
  bats reject `nested/`, `deeper/volume1/...`, `share/...` shapes. `PKG_TEST_VOLUMES_ROOT` moved
  into common.sh (`PKG_VOLUMES_ROOT`), used by preinst and postuninst.
- ADR 0026 section 6 (path table row "action lock") and section 9 (lock paragraph) added.
- Nits: boot_id recorded in the lock; comment on the double-break race in `break_lock`; README
  "Application Privileges" reworded as unconfirmed; README wording for the volume prefix.

`| spk build scripts |`
- `--untracked-files=normal` in build.sh and build.ps1; bats `release_sees_untracked_files_git_config_hides`
  runs for both scripts with `status.showUntrackedFiles=no`.
- ffmpeg `package.sh` requires `<sha>  <name>` (or `<sha> *<name>`) and that the file exists; bats
  "SOURCE.txt is refused unless the checksum line names an existing archive" (hash-only case).
- Nits: stage.sh removes its temp file on `die` (EXIT trap; bats "no temporary file is left, whether
  the stage succeeds or dies"); empty-maintainer `com..<id>` refused in `Validate` (two Go cases);
  inspect `check_desktop_keys` compares the entry key set (bats "a main menu entry with a missing or
  an extra key fails"); toolkit/README reflowed and the history note now says 69 -> 91 checks.

`| spk inspect |`
- Duplicate check, member order and licence count take the whole name (`member_names`,
  `file_member_names`); bats "member names with blanks are taken whole".
- First size byte 0x81-0xfe is invalid (`-1`); bats "a size field with a first byte 0x81 to 0xfe is
  invalid".
- `--expected` grep exit status checked (`read_expected_rules`: 1 = no rule, 2 = unreadable).
- Nits: header wording on partial extraction, `dirname` in the tool list (script and README),
  `sha256sum` failure in `--members` fails the inspection ("--members fails when a member cannot be
  hashed"), README dash wording (spaced hyphens removed), "69 checks" note, ADR 0026 wording "an ELF
  ABI note, if present".

## Findings

None.

## History

- Round 1: `SPK-13-pre-scripts-test.md`, `SPK-13-pre-inspect-test.md` (88 checks per SPK).
- Round 2: this report; 91 checks per SPK.
