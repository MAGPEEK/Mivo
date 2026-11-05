# SPK-17 test report

Status: FAIL (round 2 of 3; round 1 aborted)

## Gates

| Gate | Result |
|---|---|
| spk-script-test bats (lifecycle, package-files, start-stop-status; busybox sh, bash --posix, bash, dash) | PASS, 381/381 ok |
| spk-inspect bats | PASS, 61/61 ok (exit 0) |
| shellcheck (package scripts `-s sh`, test scripts, inspect/smoke scripts) | PASS |
| `go test ./packaging/synology/...` | PASS (4 packages) |
| SPK build (`build.sh --test 0.1.0-alpha.3`, x86_64 + armv8 + armv7) | PASS, 98 s |
| `inspect-spk.sh` on the three real SPKs | PASS each: 95 checks, 0 failed, 2 warnings (known "no GNU ABI note" for ffmpeg/ffprobe) |
| Emulated smoke x86_64, scenario 1 (fresh install) | FAIL |
| Emulated smoke x86_64, scenario 2 (alpha.2 upgrade) | FAIL (harness defect; conversion itself ok) |

## Failures

1. Smoke scenario 1, `packaging/synology/test/smoke/smoke.sh:402` (`check_first_start_state`):
   `GET /api/v1/folder-roots` returns 503. Server log:
   `listFolderRoots: library management is not configured: unavailable`.
   `GET /api/v1/libraries` returned `{"libraries":[]}` (that part is ok). Cause: the server built
   from this base (HEAD b4e96a8, also main) is not wired for library management; the composition
   root task M3-19l (`docs/tasks/M3-19l-composition-root-library-management.md`) has no commit
   (`git grep 'Folders:' main -- cmd` is empty; `restapi/folders.go:40` hits `api.folders == nil`).
   Not a packaging defect, but it blocks the whole SPK-17 smoke (POST library, scan, playback
   smoke) and the AC "first start state". SPK-17 needs M3-19l merged first.
2. Smoke scenario 2, `packaging/synology/test/smoke/dsm.sh:463` (`cmd_upgrade`):
   `FAIL the upgrade lost /var/packages/mivo/var/data`. The alpha.2 scenario seeds an installation
   with "no database yet" (smoke.sh:467), so `var/data` holds no files and the unconditional
   "files kept in var/data" check fails. The packaging logic itself worked in that run:
   `preupgrade` recorded `share-links-before-upgrade.tsv`, `library path of share video:
   /var/packages/mivo/shares/video -> /volume1/video`, `postupgrade` exit 0,
   `config.yaml now: path:"/volume1/video", backup kept`. Fix: skip the data check when
   `$STATE_DIR/alpha2-seeded` exists (or seed a data file). The later steps of scenario 2
   (library listed with `/volume1/<share>`, which also needs M3-19l) did not run.

## Not run

- Smoke for armv8 and armv7 (QEMU): x86_64 smoke already fails; not worth the time before the fixes.
- Playback smoke, POST library, scan: blocked by failure 1.
- Node/Go/web gates: out of scope (packaging only).

## Acceptance criteria (evidence)

- Wizard without input / no `wizard_media_share`, `conf/resource` only `port-config`, preinst/postinst,
  start-stop-status browse roots, upgrade rewrite, fixtures removed: met (package-files, lifecycle,
  start-stop-status bats green; 381 ok).
- Inspect expectations (fails on `data-share` / `wizard_media_share`): met (inspect bats 61 ok; real SPKs 0 failed).
- Build for all families + inspection: met.
- Emulated smoke (both scenarios): NOT met, see failures.
- First start state `GET /api/v1/libraries` = `[]`: met at the HTTP level; web empty state and folder roots: not verifiable (503).
- Docs and ADR notes, owner device test: not part of this gate (device test is the owner's).

## Cleanup

Removed: smoke containers `spk-smoke-x86_64-native` and `...-alpha2`, scratch files, the
temporary `testdata/media` copy in the worktree. `packaging/synology/dist/` (git-ignored) holds the
rebuilt test SPKs. Could not remove (blocked by the permission classifier): the two `/tmp/spk-smoke-x86_64.*`
work directories of this run (`DjeUjq` and the newest other one); older ones stem from earlier runs.

## History

- Round 1: aborted.
- Round 2: FAIL as above.
