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
- OD-SPK-3 **A**: one prebuilt static GPL FFmpeg source for all families if possible, an own
  build for a missing family as follow-up; the exact corresponding source is attached to each
  GitHub Release with SPKs.
- OD-SPK-4 **A**: armv7 is published only if its emulated smoke run passes, labelled "tested
  under emulation only".
- OD-SPK-5 **A**: `0.1.0-alpha.2` → `0.1.0-1020`, final → `0.1.0-9000`, beta flag for
  pre-releases.
- OD-SPK-6 **A**: the wizard asks for one shared folder (read-only `data-share`), `postinst`
  writes `config.yaml`; further folders via Control Panel + `config.yaml` over SSH.
- OD-SPK-7 **A**: `$SYNOPKG_PKGTMP/transcode`, if SPK-01 confirms.
- OD-SPK-8 **A**: uninstall wizard checkbox, unchecked keeps the data.
- OD-SPK-9 **A**: package name `brand.ID` (`mivo`) for now.
- Approvals: network downloads (FFmpeg builds and sources, Docker images, the `pkgscripts-ng`
  clone and the platform environments EnvDeploy downloads, QEMU binfmt) are **approved**. The
  `CLAUDE.md` "Commands" line is **not** approved: SPK-09 documents the build command in
  `docs/development.md` and `packaging/synology/README.md` only. `CLAUDE.md` already lists
  `mivo-synology` in the agent workflow.

## Tasks

| ID | Title | Owner | Depends on | Wave |
|---|---|---|---|---|
| [SPK-01](SPK-01-dsm-package-facts.md) | Verify the DSM 7.2 package facts | mivo-synology | – | 1 |
| [SPK-02](SPK-02-ffmpeg-builds-per-arch.md) | Select and pin static FFmpeg builds per family | mivo-synology | – | 1 |
| [SPK-04](SPK-04-toolkit-walking-skeleton.md) | Package Toolkit in Docker: walking skeleton | mivo-synology | – | 1 |
| [SPK-08](SPK-08-package-icons.md) | Generated, name-free package icons | mivo-synology | – | 1 |
| [SPK-03](SPK-03-package-metadata.md) | Version mapping, families, templates and env files (Go) | mivo-synology | 01, 04 | 2 |
| [SPK-05](SPK-05-service-control-script.md) | Server service control (`start-stop-status`), runtime environment, script tests | mivo-synology | 01 | 2 |
| [SPK-06](SPK-06-wizard-lifecycle-resources.md) | Wizard, lifecycle scripts, privilege and resources | mivo-synology | 01, 05 | 3 |
| [SPK-14](SPK-14-stage-toolkit-project.md) | `spkstage`: stage the toolkit project | mivo-synology | 02, 03, 04, 06, 08 | 4 |
| [SPK-07](SPK-07-build-pipeline.md) | CI-ready build: prebuild + toolkit in Docker | mivo-synology | 02, 04, 14 | 5 |
| [SPK-09](SPK-09-docs-and-test-protocol.md) | User/build docs and manual test protocol | mivo-synology | 07 | 6 |
| [SPK-10](SPK-10-verify-toolkit-output.md) | Verify the toolkit output | mivo-synology | 07 | 6 |
| [SPK-11](SPK-11-emulated-smoke.md) | Emulated install + playback smoke per family | mivo-synology | 07, M2-32, M2-35 | 6 |
| [SPK-12](SPK-12-owner-dsm-install-test.md) | Manual install test on the owner's Synology | mivo-tester | 09, 10, 11 | 7 |
| [SPK-13](SPK-13-publish-github-release.md) | Attach SPKs to the GitHub Release | mivo-release | 12, M2-36, release decision | 8 |

Task ids are stable identifiers, not the order: SPK-14 runs before SPK-07.

## Waves (parallel within a wave; no two tasks of a wave share a file)

1. SPK-01 ‖ SPK-02 ‖ SPK-04 ‖ SPK-08 – start now; disjoint paths (`DSM-NOTES.md`, `ffmpeg/**`,
   `toolkit/**` + `package/SynoBuildConf/**` + `package/INFO.sh`, `internal/spkicon/**`).
2. SPK-03 ‖ SPK-05 (`internal/spkmeta/**` vs `package/scripts/` + `test/**`)
3. SPK-06 (extends the script harness of SPK-05)
4. SPK-14 (`cmd/spkstage/**`)
5. SPK-07 – first complete server SPKs from one command; checkpoint: the x86_64 SPK installs
   into an emulated layout and the server binary prints its version.
6. SPK-09 ‖ SPK-10 ‖ SPK-11 (`README.md`/`TESTING.md` vs `test/inspect/**` + `toolkit/README.md`
   section vs `test/smoke/**`)
7. SPK-12 (owner's hardware) – fixes, if any, as `SPK-12b…`
8. SPK-13 (publishing)

Walking skeleton first: SPK-04 proves in wave 1 that EnvDeploy and PkgCreate.py run in Docker
Desktop and pack a placeholder payload for all three families – the riskiest part of OD-SPK-2 B.
The first SPK that runs the real server needs scripts (SPK-05/06) and staging (SPK-14), so the
end-to-end checkpoint is after SPK-07. Only SPK-11 (wired server M2-32, smoke script M2-35) and
the release gates depend on unfinished milestone-2 work.

## How the track lines up with 0.1.0-alpha.2

| Milestone-2 step | SPK step |
|---|---|
| waves 5–6 (M2-30, M2-32, M2-33, M2-34, M2-35) in progress | SPK waves 1–5 |
| M2-35 smoke script exists, Docker smoke green | SPK-11 runs `scripts/playback-smoke` against the server installed from each SPK under emulation |
| M2-36 release readiness on a clean checkout | SPK-12 tests SPKs built from the **same commit** (release candidate); the readiness report lists the SPK status (orchestrator note in M2-36) |
| release decision (OD-9), tag `v0.1.0-alpha.2`, GitHub Release by `mivo-release` | SPK-13 attaches the SPKs to that GitHub Release, possibly after the Docker release (OD-SPK-1 A) |

A packaging-only fix after the tag uses the package revision (`0.1.0-1021`), not a new server
version.

## Risks and notes

- EnvDeploy needs a chroot: SPK-04 must show that it runs in Docker Desktop (privileged
  container, toolkit in a named volume). If it cannot, SPK-04 reports BLOCKED and the owner
  decides between a Linux VM/WSL distribution as build host and revisiting OD-SPK-2.
- Platform environments are large downloads (several GB in total); they are cached in a Docker
  volume and, later in CI, in an actions cache.
- The toolkit's packing is not byte-reproducible; SPK-10 records what is (members, modes,
  checksums of members). Release assets are the files that were tested (SPK-13).
- Several DSM facts are unconfirmed (ADR 0026 "To verify"); SPK-01 and SPK-04 run first, and
  their findings may change details of later tasks. Implementers report contradictions instead
  of guessing; the architect adds them as Notes to ADR 0026 after wave 1.
- A static FFmpeg for armv7 that runs on DSM 7.2's kernels may not exist as a prebuilt; an own
  build would add a task and build time (OD-SPK-3 A).
- Emulation (QEMU) proves instruction set and behaviour, not the DSM kernel version or real
  performance; kernel compatibility is checked through the ELF ABI note and on the owner's
  x86_64 device only.
- Package Center shows `0.1.0-1020` for `0.1.0-alpha.2`; the docs and release notes explain it.
- SPKs are 100+ MB; GitHub Release assets allow that.
