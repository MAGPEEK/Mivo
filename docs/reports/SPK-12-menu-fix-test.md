# SPK-12 menu fix (INFO dsmappname) - test report

Round 1, 2026-11-04. Subject: uncommitted working tree of the worktree
`.claude/worktrees/agent-ae369a65579a3b659` (15 modified files under `packaging/synology/`).
Context: `docs/reports/SPK-12-owner-test.md` Run 2 (no Mivo entry in the DSM main menu).

**Status: PASS**

## Gates

| Gate | Command | Result |
|---|---|---|
| go vet | `go vet ./packaging/...` | clean |
| Go tests | `go test -race ./packaging/...` | ok: spkstage, recipecheck, spkicon, spkmeta |
| golangci-lint | `golangci-lint run ./packaging/...`, `golangci-lint fmt --diff ./packaging/...` | 0 issues, no format diff |
| Script bats (4 shells) | `spk-script-test:dev ... test/scripts` | 367 ok, 0 not ok (87 cases each for bash, bash --posix, busybox sh, dash, plus 19 shell-independent) |
| Build bats incl. build.ps1 parity | `spk-build-test:dev ... test/build` | 90 ok, 0 not ok (build.ps1 cases 33-42 ok) |
| Inspect bats | `spk-inspect:dev bats test/inspect` | 59 ok, 0 not ok |
| Toolkit | `bash toolkit/test.sh` | 69 ok, "all toolkit tests passed" (lint, x86_64, armv8, armv7; includes "INFO.sh writes no dsmappname without ui", "INFO.sh rejects a DSM_APP_NAME list") |
| shellcheck v0.11.0 | package scripts (`-s sh`), build/test scripts, inspect scripts, `package/INFO.sh` | exit 0 for all three invocations |
| Real build | `build.sh --test --package-revision 1 0.1.0-alpha.3` | rc 0, done in 164 s |
| Inspect --release | `inspect-spk.sh --release <spk> <family>` x3 | `93 checks, 0 failed, 2 warnings` for x86_64, armv8, armv7 |
| SPK-11 smoke | `smoke.sh <family> dist/*-<family>.spk` x3 | x86_64 28 passed / 0 failed; armv8 28 / 0 (QEMU); armv7 82 / 0 (QEMU, cortex-a9, cortex-a15, a9 worst case) |

The two inspect warnings are the known "no GNU ABI note" notes for the static `bin/ffmpeg` and
`bin/ffprobe` (kernel floor is checked by the smoke test); they are unrelated to the fix.

## Acceptance check: INFO and ui/config

Read from the freshly built SPKs (`mivo-0.1.0-alpha.3-r1-{x86_64,armv8,armv7}.spk`), identical in all three:

```
package="mivo"
version="0.1.0-1031"
dsmuidir="ui"
dsmappname="com.magpeek.mivo"
```

`package.tgz` `ui/config` has exactly one entry, key `com.magpeek.mivo` (type url, port 8460,
allUsers true). INFO `dsmappname` equals that key. `inspect-spk.sh` checks this itself (new checks
`INFO dsmuidir="ui"`, `INFO dsmappname=...`, `ui/config: the entry key is INFO dsmappname`, all ok on the three files).

Build hashes (test build, NOT-FOR-RELEASE):

```
dd26f4ef3be0f1bcbb0e0c73fd1ed9a134743539790ab32cb78088cc18dd9c35  mivo-0.1.0-alpha.3-r1-x86_64.spk
abd9146babb7f223e0b4311cba336b69fb54cced00bdacf5e4f6b9156831537f  mivo-0.1.0-alpha.3-r1-armv8.spk
54fa6b1f834c4eafd7a01256da6befdafec897c63ea2d8a6adb6b2e230b731fd  mivo-0.1.0-alpha.3-r1-armv7.spk
```

## Not covered

- The fix itself (menu entry visible in DSM 7.4.1) can only be confirmed on the DS925+ (owner re-test of A13).
- Web, root-module Go tests and layer rules were out of scope (change is confined to `packaging/`).

## History

- Round 1: PASS, no failures.
