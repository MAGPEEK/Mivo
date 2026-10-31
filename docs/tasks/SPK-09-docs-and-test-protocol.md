# SPK-09 Synology documentation and manual test protocol
Owner: mivo-synology
Depends on: SPK-07
Files/areas: `packaging/synology/README.md` (new), `packaging/synology/TESTING.md` (new);
minimal touches elsewhere: `README.md` (one "Synology" line in the installation part),
`docs/development.md` (one subsection "Synology package"). **`CLAUDE.md` is not changed** (owner
decision 2026-10-31).

## Goal
A Synology user can install, configure, upgrade and remove the media-server package from the
docs alone, a contributor can build it with the official toolkit, and the owner has a
step-by-step protocol for the hardware test.

## Acceptance criteria
- [ ] `packaging/synology/README.md` states up front that the package runs the media server on
      the NAS as a DSM service (web app in the browser on port 8460), not a player app on DSM.
      It covers: supported DSM (7.2+) and families, how to find the NAS's family (Synology's CPU
      list / Info Center), download and checksum check, Package Center > Manual Install including
      the third-party/unsigned warning (SPK-01 Q12), the wizard, granting more shared folders
      (Control Panel path, package system user, "Read"), editing `config.yaml` over SSH (path,
      example, restart), which settings the package fixes (paths, tools, port 8460) and why, the
      "Open" button and firewall service, the LAN-only guard and the warning about DSM's reverse
      proxy / application portal before milestone 7, logs (location, Package Center view per
      SPK-01), upgrade and uninstall data behaviour, downgrades unsupported, the Package Center
      version number (`0.1.0-1020` = `0.1.0-alpha.2`), limits (software transcoding only, 1 GB
      RAM, armv7 Direct Play/Remux, armv7/armv8 "tested under emulation only" where it applies),
      FFmpeg licence and where its source is, troubleshooting (port in use, share permission
      denied, status "broken", start failure message).
- [ ] The README has a "Build" section: prerequisites (Docker Desktop, privileged containers
      allowed), `build.ps1` / `build.sh` usage, the two steps (prebuild, toolkit packing), the
      named volume with the platform environments (size, how to reset it), build inputs and
      outputs, the pinned `pkgscripts-ng` commit and how to update it.
- [ ] `docs/development.md` gets a "Synology package" subsection with the build command (PowerShell
      and bash), the script tests, shellcheck and the SPK inspection command (SPK-10).
- [ ] `packaging/synology/TESTING.md`: the manual protocol of SPK-12 as numbered steps, each with
      the action, the expected observation and a result column; plus the data the owner reports
      back (model, platform, DSM version, SPK checksum).
- [ ] `README.md` links to `packaging/synology/README.md`.
- [ ] Every relative link in the changed Markdown files resolves (tester greps `](` targets
      against the tree); docs are English; the task's change set does not include `CLAUDE.md`.
- [ ] Gates: docs-only change; `golangci-lint run`, `go test ./...` and the script tests still
      pass unchanged.

## Notes
- These docs name the product and are on the ADR 0002 rename checklist (Notes ADR 0026); keep the
  name to prose and derived values, no new structural identifiers.
- Keep facts in one place: link to ADR 0018 Notes M2-31 for the guard details, to
  `packaging/synology/ffmpeg/README.md` for FFmpeg sources and to
  `packaging/synology/toolkit/README.md` for toolkit findings.
- Describe what SPK-01 confirmed; anything still unconfirmed is phrased as "may" and listed in
  TESTING.md as something to observe.
- Do not run at the same time as M2-36 (both edit `README.md` and `docs/development.md`).
