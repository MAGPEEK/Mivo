# SPK-11 Emulated install and playback smoke of every SPK
Owner: mivo-synology (writes and runs the harness; `mivo-tester` re-runs it as the gate)
Depends on: SPK-07, M2-32 (wired server), M2-35 (`scripts/playback-smoke`)
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
      `shares/<share>` (the corpus `testdata/media` mounted **read-only**), sets the `SYNOPKG_*`
      variables SPK-01 documents, runs `postinst` with `wizard_media_share=<share>`, then
      `start-stop-status start`, as a non-root user.
- [ ] Before installing, `packaging/synology/test/inspect/inspect-spk.sh` (SPK-10) passes for the
      SPK under test (if SPK-10 is not done yet, run it once SPK-10 lands and add the result).
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
