# SPK-11 test report (mivo-tester)

Round 1. Result: **PASS**. Base 804b256 plus the uncommitted harness `packaging/synology/test/smoke/`.
SPKs: `packaging/synology/dist/mivo-0.1.0-alpha.2-{x86_64,armv8,armv7}.spk`. No code edited.

## Gates

| Gate | Command | Result |
|---|---|---|
| shellcheck | `koalaman/shellcheck:v0.11.0` on smoke.sh, dsm.sh, qemu-shim.sh | exit 0, no findings |
| hadolint | `hadolint/hadolint < smoke/Dockerfile` | exit 0, no findings |
| inspect-spk.sh --release | in `spk-inspect:dev` (as smoke.sh does) | x86_64, armv8, armv7: `67 checks, 0 failed, 0 warnings` |
| Go/web gates | not run: no product code changed (`git status`: only `?? packaging/synology/test/smoke/`) | n/a |

## Smoke runs (commands exactly as in README.md)

| Family | CPU | Summary | Playback chromium / firefox | FFmpeg line | SPK size | Whole run |
|---|---|---|---|---|---|---|
| x86_64 | native (host) | `SUMMARY x86_64: 28 passed, 0 failed`, exit 0 | 30/30 in 4 s (scan 286 ms) / 28/28 in 3 s (scan 281 ms) | ffmpeg version 9.0.2-static1 | 21422080 B | 22 s (wall 28.6 s) |
| armv8 | cortex-a53 | `SUMMARY armv8: 28 passed, 0 failed`, exit 0 | 30/30 in 24 s (scan 361 ms) / 28/28 in 21 s (scan 331 ms) | ffmpeg version 9.0.2-static1 | 18964480 B | 82 s |
| armv7 | cortex-a9 | `SUMMARY armv7: 55 passed, 0 failed`, exit 0 (both CPUs) | 30/30 in 28 s (scan 365 ms) / 28/28 in 24 s (scan 342 ms) | same | 16476160 B | 89 s |
| armv7 | cortex-a15 | (same run) | 30/30 in 29 s (scan 361 ms) / 28/28 in 25 s (scan 347 ms) | same | 16476160 B | 91 s (wall 186 s both) |

No transcode timeouts occurred, so the slow-timeout rerun was not triggered. Timings are not performance evidence (QEMU).

## Explicit checks

- Upgrade, uninstall-keep-data, reinstall, uninstall-delete-data: PASS lines present in all four CPU runs
  (e.g. x86_64: `upgrade (stop, preupgrade, preuninst, postuninst, preinst, postinst, postupgrade)`,
  `upgrade: catalogue kept (28 items ... 0 added)`, `uninstall keeping the data`,
  `reinstall (keeps config.yaml)`, `reinstall: catalogue kept`, `uninstall deleting the data`; var/home emptied).
- Shim scope (armv8, kept container, then `dsm.sh install`): the 13 files under `target/` have the same SHA-256 as
  `package.tgz` of the SPK. In `scripts/` eight files equal the SPK's; only `common.sh` differs, and its SHA-256
  (619bafe9...) equals `sha256(SPK common.sh + qemu-shim.sh)` exactly. Shim log line: `QEMU user mode: qemu-shim.sh
  appended to the installed scripts/common.sh`. x86_64 log has no such line (scripts unchanged).
- SPK unchanged: sha256 before/after the `--keep` run identical (armv8 `a3215dca...`); all three verified with
  `sha256sum -c` after all runs (mounted `:ro`).
- Negative test: x86_64 run with the server `kill -KILL`ed after the first status check: exit 1,
  `chromium-desktop: 1 passed, 29 failed` (`EOF`), firefox `reach the server` failed, `after the stop` check failed,
  `SUMMARY x86_64: 25 passed, 3 failed`.
- Cleanup: no `spk-smoke-*` containers remain; images `spk-smoke:{x86_64,armv8,armv7}` kept.

## Accepted deviations (documented in README.md, "Deviations from DSM and why")

1. QEMU `/proc/<pid>/exe` shim: documented with evidence; limited to the installed `common.sh` of armv8/armv7, original
   check runs first, SPK untouched (verified above), x86_64 unchanged.
2. x86_64 native: documented; probe shows `uname` = `Linux 6.18.33.2-microsoft-standard-WSL2 x86_64` for package
   processes, `uname through qemu-x86_64: Linux 3.10.108 x86_64` for the version commands only; filter active
   (`getrandom=ENOSYS statx=ENOSYS (54 syscalls fail with ENOSYS)`). armv8 reports `Linux 4.4.302 aarch64`, armv7
   `Linux 3.10.108 armv7l`.

## Acceptance criteria

| # | Criterion | Status | Evidence |
|---|---|---|---|
| 1 | One documented command per family, container of the platform, unpack to target, var/tmp/home/shares, corpus read-only, SYNOPKG_*, preinst/postinst with wizard_media_share, prestart + start as non-root | met | README commands; install/prestart/start PASS in all runs; share snapshot unchanged and read-only |
| 2 | inspect-spk.sh --release passes before install | met | `67 checks, 0 failed` x3; first step of each run |
| 3 | kernelsim with 3.10 / 4.4 list, uname = DSM kernel | **partly met** | armv8 (4.4.302) and armv7 (3.10.108) fully. x86_64: syscall filter applies, but package processes see the host `uname` (6.18.33.2 WSL2) because it runs natively (no binfmt for x86_64 on x86_64); only the version commands run via `qemu-x86_64 -r 3.10.108`. Accepted, documented, stricter for the Go runtime |
| 4 | armv7 twice, cortex-a9 and cortex-a15 | met | 55 passed, 0 failed; both CPU runs, no SIGILL |
| 5 | `--version`, ffmpeg/ffprobe -version exit 0, start 0, status 0, port 8460 published | met | `mivo --version: exit 0: Mivo 0.1.0-alpha.2 (go1.27.3)`; versions PASS; `port 8460 published as 127.0.0.1:8460` PASS |
| 6 | playback-smoke passes every case; armv7 timeout rerun option | met | 30/30 + 28/28 on all CPUs; `--slow-case-timeout` / `SMOKE_SLOW_CASE_TIMEOUT` documented (not triggered) |
| 7 | stop exits 0 within 20 s, status 3, no ffmpeg, sessions empty, share unchanged, no log error | met | `stop finished in 1.02 s (budget 20 s)`, `status exits 3`, `no server or FFmpeg process left`, sessions empty, `share video unchanged (55 entries) and read-only`, `no error in mivo.log` |
| 8 | Per-family report: FFmpeg line, summary, scan and run durations, SPK size | met | INFO and playback lines (table above) |
| 9 | shellcheck no findings | met | exit 0 (hadolint also clean) |
| 10 | No product code changed | met | `git status`: only `packaging/synology/test/smoke/` untracked |

## History
- Round 1: PASS.

REPORT
status: PASS
task: SPK-11
files: docs/reports/SPK-11-test.md
tests: shellcheck 0 findings; hadolint 0 findings; inspect-spk.sh --release 67 checks/0 failed x3; smoke.sh x86_64 28 passed 0 failed (28 s), armv8 28/0 (88 s), armv7 55/0 (186 s, a9+a15); negative (server killed) exit 1
findings:
- x86_64 uname criterion partly met (native run, host uname; filter active; accepted)
- shim touches only installed common.sh (hash = SPK common.sh + shim); SPK sha256 unchanged
next: proceed to mivo-reviewer
