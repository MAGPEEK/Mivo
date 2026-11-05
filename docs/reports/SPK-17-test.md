# SPK-17 test report

Status: FAIL (round 3 of 3; HEAD 527ae55 rebased on main incl. M3-19l)

## Gates

| Gate | Result |
|---|---|
| spk-script-test bats (busybox sh, bash --posix, bash, dash) | PASS, 381/381 ok |
| spk-inspect bats | PASS, 61/61 ok |
| shellcheck (package scripts `-s sh`, test scripts) | PASS |
| `go test ./packaging/synology/...` | PASS (4 packages) |
| SPK build `build.sh --test 0.1.0-alpha.3` (x86_64, armv8, armv7) | PASS, 143 s |
| `inspect-spk.sh` on the three SPKs | PASS each: 95 checks, 0 failed, 2 warnings (known ffmpeg ABI-note warnings) |
| Smoke x86_64 | 27 passed, 1 failed |
| Smoke armv8 (cortex-a53, QEMU) | 27 passed, 1 failed |
| Smoke armv7 (cortex-a9 only, QEMU) | 27 passed, 1 failed |

## What passed in every family

Fresh install: `GET /api/v1/libraries` = `[]`, folder root `/volume1`, installKind `synology`;
`POST /api/v1/libraries` with the corpus share; scan; playback smoke chromium-desktop 30/30 and
firefox-desktop 28/28. Alpha.2 upgrade scenario: `preupgrade` records the link, `postupgrade`
rewrites the path, library listed with `/volume1/video` (all steps PASS, including the data
check that failed in round 2).

## Failure (same in all three families)

`packaging/synology/test/smoke/smoke.sh:371-372` (`check_kept_catalogue`), step "upgrade"
of scenario 1 (plain upgrade of the fresh install):
`FAIL <family> upgrade: catalogue not kept or scan unfinished`. The library JSON shows
`"itemCount":22` and scan counts `"unchanged":28,"added":0`. The check requires
`unchanged == itemCount`, but the library now also scans extras (`scanExtras:true`), so
the scan counts 28 files for 22 items. The catalogue is in fact kept (added 0, nothing
failed, state idle); this is a harness assumption broken by the new library options, not a
packaging defect. Fix: in `check_kept_catalogue` require `added == 0`, `failed == 0` and
`unchanged >= items` (or compare against the first scan's `unchanged` from before the upgrade).

## Not covered

armv7 ran only `cortex-a9`, not `cortex-a15` and the NEON-off worst case.
Containers and `/tmp/spk-smoke-*` removed; nothing committed. `testdata/media` was
regenerated (untracked/ignored).

## History

- Round 1: aborted.
- Round 2: FAIL: smoke blocked by missing M3-19l wiring (503 folder-roots); upgrade data check defect.
- Round 3: all packaging gates PASS; both round 2 defects fixed; one remaining smoke
  harness assertion (above) fails in every family.

REPORT
status: FAIL
task: SPK-17
files: docs/reports/SPK-17-test.md
tests: bats 381/381, inspect bats 61/61, shellcheck, go test packaging/synology, 3 SPKs built, inspect 0 failed x3; smoke 27 pass / 1 fail per family
findings:
- smoke.sh:371 check_kept_catalogue asserts unchanged == itemCount; extras make unchanged 28 vs itemCount 22 (added 0). Relax to added==0, failed==0, unchanged>=items.
next: fix the harness assertion (hard limit of 3 tester runs reached; orchestrator to decide / ask owner)
