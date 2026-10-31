# SPK-01 Verify the DSM 7.2 package facts
Owner: mivo-synology
Depends on: none
Files/areas: `packaging/synology/DSM-NOTES.md` (new) only

## Outcome (2026-10-31, wave 1)
Implemented: 15 questions answered, 22 "Impact on ADR 0026" items. The accepted ones and the
orchestrator decisions on them (armv7 `arch` list, classic JSON wizard, upgrade test for the
version scheme) are recorded in ADR 0026 "Notes (SPK-01, SPK-02, SPK-04)" and folded into SPK-03,
SPK-05, SPK-06, SPK-07, SPK-10, SPK-12 and SPK-14.

## Goal
Every DSM behaviour that ADR 0026 marks *(verify, SPK-01)* is answered from Synology's DSM 7.2.2
Package Developer Guide or the `pkgscripts-ng` sources, so the tasks that package the media
server as a DSM service build on facts instead of assumptions. How the toolkit itself runs
(privileges, platforms, helpers, the `arch` it writes) is SPK-04's part, not this task's.

## Acceptance criteria
- [ ] `packaging/synology/DSM-NOTES.md` answers each question below in its own numbered section.
      Every answer has a status `confirmed` or `unconfirmed` and a source: guide URL with the
      section title, or `pkgscripts-ng` file path plus commit SHA on branch `DSM7.2`.
      `unconfirmed` answers state the assumption the tasks use and which later check (SPK-10,
      SPK-11 or SPK-12) confirms it.
- [ ] Questions:
  1. SPK container: archive format (plain tar? which flavour), compression, required and
     optional members, whether member order matters; allowed compressions of `package.tgz`
     (gzip, xz).
  2. INFO syntax: line format and quoting/escaping; version format and **how DSM compares
     versions** (numeric per dot segment? build number?) – decides OD-SPK-5; maximum lengths;
     beta flag key; German description key; `changelog`; `thirdparty` (needed?);
     `checkport`, `startable`, `ctl_stop`, `support_move` semantics; `adminport` /
     `adminprotocol` / `adminurl` behaviour of the "Open" button.
  3. `arch` values: are `x86_64`, `armv8`, `armv7` accepted family values on DSM 7.2, and which
     DSM 7.2 platforms does each cover; `os_min_ver` build number of DSM 7.2 GA.
  4. Icons: file names, pixel sizes, format.
  5. Scripts: which user runs each script on DSM 7, which `/bin/sh` DSM uses, available
     environment variables per script (`SYNOPKG_PKGNAME`, `SYNOPKG_PKGDEST`, `SYNOPKG_PKGVAR`,
     `SYNOPKG_PKGTMP`, `SYNOPKG_PKGHOME`, `SYNOPKG_PKG_STATUS`, `SYNOPKG_OLD_PKGVER`,
     `SYNOPKG_TEMP_LOGFILE`, wizard keys), how a script shows an error message in Package
     Center, time limits for `start`/`stop`, whether extra files in `scripts/` (`package.env`,
     a shared `common.sh`) are allowed and where they are at run time, what runs at boot.
  6. `conf/privilege` schema: `run-as: package`, package user name (= package name?), options
     we must not use.
  7. `conf/resource`: `data-share` (schema, ro/rw, templating of wizard values, behaviour for a
     non-existent share, on upgrade with an empty upgrade wizard, on uninstall, existence of
     `/var/packages/<pkg>/shares/<share>`), `port-config` (schema, where the `.sc` file lives,
     its format, `port_forward` semantics, interaction with `checkport`).
  8. Package directories `var`, `tmp`, `home`, `etc`, `target`: real locations (data volume or
     system partition), owner and modes, lifetime on upgrade, uninstall and reboot; whether
     `tmp` is ever cleaned while the package runs.
  9. Wizard: `WIZARD_UIFILES` file names (install/upgrade/uninstall, language suffix for
     German), JSON schema for text fields, checkboxes and validators, dynamic wizards
     (`install_uifile.sh`), how wizard values reach the scripts.
  10. Uninstall: does DSM delete `var`, and can a package keep it on request (decides OD-SPK-8).
  11. Package log: which file, if any, Package Center shows for a third-party package.
  12. Manual install of an unsigned SPK on DSM 7.2.2: trust-level setting, warnings, steps.
  13. Oldest kernel version per family on DSM 7.2, CPU features of the DSM 7.2 armv7 platforms
      (VFPv3, NEON), typical RAM of armv7/armv8 models.
  14. Container Manager availability per family (ADR 0026 context statement).
  15. Can QuickConnect or the DSM reverse proxy expose a package port, and with which client
      address does the request then arrive (LAN-guard caveat).
- [ ] A final section "Impact on ADR 0026" lists every answer that contradicts or sharpens a
      statement of ADR 0026 or a task file (SPK-03, SPK-05, SPK-06, SPK-08, SPK-14), with the
      proposed wording; the same list goes into the REPORT findings.
- [ ] Tests: none (research). The tester checks that all 15 questions are answered with a status
      and a source, and that every link resolves (HTTP 200 or a pinned GitHub blob URL).
- [ ] Gates: no code changed; `golangci-lint run` and `go test ./...` unaffected.

## Notes
- Prefer the guide and the toolkit sources (`include/pkg_util.sh`, `PkgCreate.py`, example
  packages in the Synology developer guide). Community sources (spksrc framework files,
  SynoCommunity packages) may support an answer but are marked as such; they are not
  `confirmed` on their own.
- Do not edit ADR 0026 or the task files yourself: wave 1 runs in parallel (SPK-02 and SPK-04
  also produce findings). The architect appends "Notes (SPK-01)" to ADR 0026 from your REPORT.
- Read-only research; downloads limited to cloning `pkgscripts-ng` into the scratchpad (approved
  network access).
