# Track SPK "Synology DSM package" – overview

**What is built**: a native Synology package that installs and runs the **media server** as a
DSM service – the `mivo` server binary with the embedded web app, the library scanner and the
bundled FFmpeg/ffprobe for probing, remux and transcode, listening on port 8460. Users open it
in a browser from other devices (Package Center's "Open" button points there). It is **not** a
client or player app for DSM, and no task of this track builds one.

Scope (owner decisions 2026-10-31, ADR 0026 Accepted): DSM 7.2+, one SPK per architecture family
(x86_64, armv8, armv7), the same server binary as the Docker image plus a bundled static FFmpeg
per family, **built with Synology's official Package Toolkit `pkgscripts-ng`** (EnvDeploy +
PkgCreate.py, branch DSM7.2) in a privileged Linux container on the Windows dev machine. Binaries
are prebuilt outside the chroot; the toolkit only packs them. Runs in parallel to milestone 2;
the SPKs are attached to the **0.1.0-alpha.2** GitHub Release when ready. Implementer:
`mivo-synology` (`.claude/agents/mivo-synology.md`).

Not in scope: hardware transcoding, HTTPS, DSM desktop app / SSO, reverse-proxy integration,
migration from Docker, package signing, a package source feed, the GitHub Actions workflow
(the build is CI-ready; the workflow comes with the general CI task), QPKG.

Work area: `packaging/synology/**`. Touches elsewhere are named in the task files: `README.md`
and `docs/development.md` (one section each, SPK-09), `docs/reports/**` (SPK-12, SPK-13).
`CLAUDE.md` is **not** changed (the "Commands" line was not approved). **No server code
changes**; if a task finds it needs one, it stops and reports (new task for `mivo-backend`).
SPK-09 and M2-36 both edit `README.md` and `docs/development.md`: never run them at the same
time (SPK-09 after M2-36, or M2-36 rebased on SPK-09).

## Decisions of the project owner (2026-10-31)

- ADR 0026 **Accepted**; unconfirmed facts stay marked *verify* (SPK-01, SPK-02, SPK-04).
- OD-SPK-1 **A**: the Docker release goes out on schedule; the SPKs are attached to the same
  GitHub Release later.
- OD-SPK-2 **B**: official Package Toolkit `pkgscripts-ng` (EnvDeploy + PkgCreate.py) – not the
  recommended own assembler. The own assembler and spksrc are rejected alternatives (ADR 0026
  §2). The former SPK-04 "own deterministic archive writer" is replaced by the toolkit walking
  skeleton; SPK-10 now verifies the toolkit's output; staging moved into the new SPK-14.
- OD-SPK-3 **revised (owner, 2026-10-31, after SPK-02)**: no prebuilt meets ADR 0026 §4
  (jellyfin/BtbN are glibc-dynamic, need Linux ≥ 4.18, have no armv7; jellyfin's GPL build links
  libfdk-aac through a patched `configure`, which is non-free upstream; johnvansickle is 7.0.2,
  has no source and needs VFPv4). Decision: **own static FFmpeg build for all three families**
  (SPK-15, wired in by SPK-16). The exact corresponding source of FFmpeg and every linked library
  is attached to each GitHub Release with SPKs. Until SPK-16, `sources.lock` holds `interim`
  pins for pipeline work only; **SPK-13 refuses while any `interim` entry exists**.
- OD-SPK-4 **A**: armv7 is published only if its emulated smoke run passes, labelled "tested
  under emulation only".
- OD-SPK-5 **A**: `0.1.0-alpha.2` → `0.1.0-1020`, final → `0.1.0-9000`, beta flag for
  pre-releases.
- OD-SPK-6 **A**: the wizard asks for one shared folder (read-only `data-share`), `postinst`
  writes `config.yaml`; further folders via Control Panel + `config.yaml` over SSH.
- OD-SPK-7 **A**: `$SYNOPKG_PKGTMP/transcode`, if SPK-01 confirms.
- OD-SPK-8 **A**: uninstall wizard checkbox, unchecked keeps the data.
- OD-SPK-9 **A**: package name `brand.ID` (`mivo`) for now.
- Orchestrator decisions on the wave-1 findings (ADR 0026 Notes):
  - (a) the armv7 SPK uses **`arch="armv7 armada38x monaco"`** and targets ARMv7 hard-float with
    VFPv3(-D16), NEON only through run-time detection, no VFPv4 assumption;
  - (b) wizards use the **classic JSON `WIZARD_UIFILES`** (community practice on DSM 7.2); the Vue
    wizard is the fallback only (needs `os_min_ver` `7.2-72806`); SPK-12 verifies on the owner's
    device;
  - (c) OD-SPK-5 stays; SPK-12 adds an upgrade test with dummy SPKs (`0.1.0-9000` → `0.1.1-1010`,
    `0.1.0-1020` → `0.1.0-1030`); if DSM refuses, the owner gets the alternative of a globally
    monotonic build number.
- From the wave-1 review: **`adminurl` is omitted** (decision in ADR 0026 Notes; `INFO.sh` is
  aligned in an SPK-04 rework round before SPK-14); **NEON on armada38x/monaco is unconfirmed** –
  the conservative armv7 target (VFPv3-D16, NEON only run-time detected) is binding, and SPK-15
  corrects the unsourced NEON claim in `ffmpeg/README.md`; **the toolkit's tar flavour is
  unconfirmed** (only `tar cf` known) – SPK-10 records it.
- Facts from wave 1 that change tasks: `prestart`/`prestop` must exit 0;
  `data-share` creates missing shares (`preinst` must reject them); `.sc` needs `desc` and lives at
  `port_conf/<id>.sc`; DSM never deletes `var` (OD-SPK-8 A works) and keeps `tmp` until upgrade
  (OD-SPK-7 A confirmed); script output goes to `/var/log/packages/<id>.log`; no `--no-sign`
  (no signing at all); toolkit success is decided by log grepping; `SynoBuildConf/depends` must be
  ASCII; toolkit needs `--privileged` or `--cap-add SYS_ADMIN` (plus `apparmor=unconfined` on
  AppArmor hosts, unverified); platforms v1000 / rtd1619b / alpine; oldest DSM 7.2 kernels x86_64
  3.10.108, armv8 4.4.302, armv7 3.10.108.
- Approvals: network downloads (FFmpeg builds and sources, Docker images, the `pkgscripts-ng`
  clone and the platform environments EnvDeploy downloads, QEMU binfmt) are **approved**. The
  `CLAUDE.md` "Commands" line is **not** approved: SPK-09 documents the build command in
  `docs/development.md` and `packaging/synology/README.md` only. `CLAUDE.md` already lists
  `mivo-synology` in the agent workflow.

## Tasks

| ID | Title | Owner | Depends on | Wave |
|---|---|---|---|---|
| ID | Title | Owner | Depends on | Wave | Status |
|---|---|---|---|---|---|
| [SPK-01](SPK-01-dsm-package-facts.md) | Verify the DSM 7.2 package facts | mivo-synology | – | 1 | implemented |
| [SPK-02](SPK-02-ffmpeg-builds-per-arch.md) | Select and pin FFmpeg builds per family (interim pins) | mivo-synology | – | 1 | implemented |
| [SPK-04](SPK-04-toolkit-walking-skeleton.md) | Package Toolkit in Docker: walking skeleton | mivo-synology | – | 1 | implemented; rework (`adminurl`, tar-flavour note) in wave 2 |
| [SPK-08](SPK-08-package-icons.md) | Generated, name-free package icons | mivo-synology | – | 1 | implemented |
| [SPK-03](SPK-03-package-metadata.md) | Version mapping, families, templates and env files (Go) | mivo-synology | 01, 04 | 2 | |
| [SPK-05](SPK-05-service-control-script.md) | Server service control (`start-stop-status`), runtime environment, script tests | mivo-synology | 01 | 2 | |
| [SPK-15](SPK-15-own-static-ffmpeg-build.md) | Own static FFmpeg build for x86_64, armv8, armv7 | mivo-media | – | 2 (long; runs alongside 2–5) | |
| [SPK-06](SPK-06-wizard-lifecycle-resources.md) | Wizard, lifecycle scripts, privilege and resources | mivo-synology | 01, 05 | 3 | |
| [SPK-14](SPK-14-stage-toolkit-project.md) | `spkstage`: stage the toolkit project | mivo-synology | 02, 03, 04, 06, 08 | 4 | |
| [SPK-07](SPK-07-build-pipeline.md) | CI-ready build: prebuild + toolkit in Docker | mivo-synology | 02, 04, 14 | 5 | |
| [SPK-16](SPK-16-use-own-ffmpeg-in-spk-build.md) | Switch the SPK build to the own FFmpeg build | mivo-synology | 07, 15 | 6 | |
| [SPK-09](SPK-09-docs-and-test-protocol.md) | User/build docs and manual test protocol | mivo-synology | 07 | 6 | |
| [SPK-10](SPK-10-verify-toolkit-output.md) | Verify the toolkit output | mivo-synology | 07 | 6 | |
| [SPK-11](SPK-11-emulated-smoke.md) | Emulated install + playback smoke per family | mivo-synology | 07, 10, 16, M2-32, M2-35 | 7 | |
| [SPK-12](SPK-12-owner-dsm-install-test.md) | Manual install test on the owner's Synology | mivo-tester | 09, 10, 11, 16 | 8 | |
| [SPK-13](SPK-13-publish-github-release.md) | Attach SPKs to the GitHub Release | mivo-release | 12, 16, M2-36, release decision | 9 | |

Task ids are stable identifiers, not the order: SPK-14 runs before SPK-07, SPK-15 starts in
wave 2.

## Waves (parallel within a wave; no two tasks of a wave share a file)

1. SPK-01 ‖ SPK-02 ‖ SPK-04 ‖ SPK-08 – **implemented** (results in ADR 0026 Notes).
2. SPK-03 ‖ SPK-05 ‖ SPK-15 ‖ SPK-04 rework (`internal/spkmeta/**` vs `package/scripts/` +
   `test/**` vs `ffmpeg/build/**` + `ffmpeg/README.md` vs `package/INFO.sh` + `toolkit/**`).
   SPK-15 is long and may run on through waves 3–5; it shares no file with them.
3. SPK-06 (extends the script harness of SPK-05)
4. SPK-14 (`cmd/spkstage/**`, `package/INFO.sh`)
5. SPK-07 – first complete server SPKs from one command (interim FFmpeg); checkpoint: the x86_64
   SPK unpacks, INFO is right and the server binary prints its version.
6. SPK-16 ‖ SPK-09 ‖ SPK-10 (`ffmpeg/sources.lock`, `fetch.sh`, `ffmpeg/README.md`, build
   scripts vs `README.md`/`TESTING.md`/`docs/development.md` vs `test/inspect/**` +
   `toolkit/README.md` section). SPK-16 waits for SPK-15.
7. SPK-11 (emulated smoke with the own FFmpeg and the old-kernel syscall surface)
8. SPK-12 (owner's hardware, including the version-scheme and wizard checks) – fixes, if any, as
   `SPK-12b…`
9. SPK-13 (publishing; refuses interim FFmpeg)

The toolkit risk of OD-SPK-2 B is retired: SPK-04 packed placeholder SPKs for all three families
in Docker Desktop. The biggest remaining risks are the own FFmpeg build (SPK-15) and the
unconfirmed DSM behaviour that only SPK-12 can check. Only SPK-11 (M2-32, M2-35) and the release
gates depend on unfinished milestone-2 work.

## How the track lines up with 0.1.0-alpha.2

| Milestone-2 step | SPK step |
|---|---|
| waves 5–6 (M2-30, M2-32, M2-33, M2-34, M2-35) in progress | SPK waves 2–6 |
| M2-35 smoke script exists, Docker smoke green | SPK-11 runs `scripts/playback-smoke` against the server installed from each SPK under emulation |
| M2-36 release readiness on a clean checkout | SPK-12 tests SPKs built from the **same commit** (release candidate); the readiness report lists the SPK status (orchestrator note in M2-36) |
| release decision (OD-9), tag `v0.1.0-alpha.2`, GitHub Release by `mivo-release` | SPK-13 attaches the SPKs to that GitHub Release, possibly after the Docker release (OD-SPK-1 A) |

A packaging-only fix after the tag uses the package revision (`0.1.0-1021`), not a new server
version.

## Risks and notes

- The toolkit runs in Docker Desktop with `--privileged` (or `--cap-add SYS_ADMIN`); on CI hosts
  with AppArmor the narrow set probably also needs `apparmor=unconfined` (unverified until the CI
  task). The volume `spk-toolkit` grows to about 24 GB for three families.
- The toolkit's packing is not byte-reproducible (`create_time`, tar times); SPK-10 records what
  is (members, modes, checksums of members). Release assets are the files that were tested
  (SPK-13).
- The own FFmpeg build (SPK-15) adds cold build time (cross-compiling FFmpeg and x264 for three
  families) and maintenance (pin updates, security fixes). It is the critical path for the first
  release SPKs; SPK-07…10 proceed with interim pins meanwhile.
- Still unconfirmed and checked only on the owner's device (SPK-12): classic wizard format on
  DSM 7.2.2, `data-share` templating, extra files in `scripts/`, version comparison, Open-button
  URL, start/stop timeouts. armada38x/monaco installs and every armv7/armv8 device behaviour stay
  unconfirmed (emulation only).
- The fdk-aac problem of jellyfin-ffmpeg also affects the Docker/Windows plan of ADR 0015 (Notes
  2026-10-31); the current Docker image (Debian FFmpeg) is not affected.
- Emulation (QEMU) proves instruction set and behaviour, not the DSM kernel version or real
  performance; kernel compatibility is checked through the ELF ABI note and on the owner's
  x86_64 device only.
- Package Center shows `0.1.0-1020` for `0.1.0-alpha.2`; the docs and release notes explain it.
- SPKs are 100+ MB; GitHub Release assets allow that.
