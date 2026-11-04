# SPK-11 Emulated install and playback smoke of every SPK
Owner: mivo-synology (writes and runs the harness; `mivo-tester` re-runs it as the gate)
Depends on: SPK-07, SPK-10, SPK-16 (gate run with the own FFmpeg; earlier runs with interim
FFmpeg count as pipeline checks only), M2-32 (wired server), M2-35 (`scripts/playback-smoke`)
Files/areas: `packaging/synology/test/smoke/**` (new: harness script and container definition)

## Goal
Each family's server SPK – server binary, FFmpeg, scripts – installs into a DSM-like layout, the
server starts as a service, scans and serves the test corpus to the playback smoke client under
emulation, so the armv8 and armv7 packages are exercised
before release even though the owner only has an x86_64 Synology (OD-9 rule, OD-SPK-4).

## Acceptance criteria
- [ ] One documented command per family (`x86_64` → `linux/amd64`, `armv8` → `linux/arm64`,
      `armv7` → `linux/arm/v7`) starts a `debian:stable-slim` container of that platform (QEMU via
      Docker Desktop), unpacks the SPK into `/var/packages/<id>/target`, creates `var`, `tmp` and
      `home` and `shares/<share>` (the corpus `testdata/media` mounted **read-only**), sets the
      `SYNOPKG_*` variables of `DSM-NOTES.md` §5 (`SYNOPKG_PKG_STATUS=INSTALL`), runs `preinst` and
      `postinst` with `wizard_media_share=<share>`, then `start-stop-status prestart` and `start`
      (DSM's order), as a non-root user.
- [ ] Before installing, `packaging/synology/test/inspect/inspect-spk.sh --release` (SPK-10) passes
      for the SPK under test.
- [ ] Kernel surface: the server and FFmpeg run under SPK-02's `kernelsim` wrapper with the family's
      old-kernel syscall list (3.10 for x86_64 and armv7, 4.4 for armv8) and `uname` reporting the
      DSM kernel, so a syscall DSM 7.2 lacks fails the smoke run (the Go runtime and the server must
      cope with `ENOSYS`).
- [ ] armv7 runs twice, with `QEMU_CPU=cortex-a9` and `cortex-a15` (armada38x/monaco versus
      alpine), so a VFPv4 dependency would show up as `SIGILL`.
- [ ] Per family: `bin/<id> --version` prints the build's version; `bin/ffmpeg -version` and
      `bin/ffprobe -version` exit 0; `start` exits 0; `status` exits 0; port 8460 is published to
      the host.
- [ ] `go run ./scripts/playback-smoke -base http://localhost:<port> -manifest testdata/media/manifest.json`
      passes for every case on x86_64 and armv8. For armv7 every case passes too; if transcode
      cases exceed the smoke script's timeouts under emulation, they are rerun with a longer
      timeout (named option or env, documented) and the durations reported – any failure other
      than a timeout is a FAIL.
- [ ] After the run: `start-stop-status stop` exits 0 within 20 s, `status` exits 3, no `ffmpeg`
      process remains in the container, `tmp/transcode/sessions` is empty, nothing was written to
      the share (read-only mount, no errors in `<id>.log`).
- [ ] REPORT per family: FFmpeg version line, smoke summary, duration of scan and run, SPK size.
- [ ] shellcheck on the harness → no findings.
- [ ] Gates: no product code changed; Go gates and script tests unaffected.

## Notes
- QEMU proves instruction set and behaviour, not DSM's kernel or real performance (ADR 0026
  consequences); do not draw performance conclusions from the timings.
- Run the corpus generation first (`go run ./scripts/gen-testmedia`); on a Linux host the corpus
  files must be readable by the container user (M2-35 note).
- If an emulated family fails for reasons outside the package (QEMU bug), document the evidence
  and mark the case; the owner decides about publishing that family (OD-SPK-4).

## Orchestrator note (2026-11-04): accepted deviations

- **x86_64 `uname`**: the x86_64 run is native (no binfmt QEMU path on an x86_64 host). The
  kernelsim 3.10 syscall filter applies to the server and FFmpeg, but `uname` shows the Docker
  host's kernel; only the version commands run under `qemu-x86_64 -r 3.10.108`. The criterion
  "uname reporting the DSM kernel" is therefore met for armv8/armv7 only; the real DSM kernel
  for x86_64 is covered by the owner's device test (SPK-12, DS925+, kernel 5.10.55).
- **QEMU process check**: under QEMU user mode `/proc/<pid>/exe` names the interpreter, so for
  armv8/armv7 the harness appends `qemu-shim.sh` to the installed copy of `scripts/common.sh`
  (never the SPK); the original check runs first. x86_64 runs the scripts unchanged.
- Upgrade, uninstall keeping data plus reinstall, and uninstall deleting data are part of the
  harness although not listed in the criteria above.
