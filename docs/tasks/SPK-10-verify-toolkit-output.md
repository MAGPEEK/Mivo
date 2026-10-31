# SPK-10 Verify the toolkit output
Owner: mivo-synology
Depends on: SPK-07
Files/areas: `packaging/synology/test/inspect/**` (new: inspection script + expected-member list),
`packaging/synology/toolkit/README.md` (new section "Output verification")

## Goal
Every server SPK the toolkit produces is checked against what ADR 0026 and SPK-01 require –
structure, modes, INFO, no leftovers from the chroot – and the reproducibility limits of the
toolkit's packing are known, so releases ship exactly what was tested.

## Acceptance criteria
- [ ] `packaging/synology/test/inspect/inspect-spk.sh <file.spk> <family>` (POSIX `sh`, runs in a
      `debian:stable-slim` container) checks and reports each item, exit 1 on any failure:
      - outer members equal the expected list (SPK-01 Q1) and include no signature file;
      - INFO: `package`, `displayname`, `version` matching the DSM version pattern, `arch` = the
        family, `os_min_ver`, beta flag for pre-releases, `adminport`, `adminprotocol`,
        `adminurl`, descriptions present;
      - `package.tgz`: `bin/<id>`, `bin/ffmpeg`, `bin/ffprobe` executable, ELF machine matching the
        family, FFmpeg licence and source notice present; nothing from the build environment
        (no toolchain files, no `SynoBuildConf`, no absolute paths, no `..`);
      - `scripts/` members executable, LF line endings; `conf/privilege` without root; every JSON
        member parses; no unreplaced `@NAME@` token anywhere; owners and modes listed.
- [ ] The script passes on all three SPKs of SPK-07; output in the REPORT.
- [ ] Reproducibility: two SPK-07 builds of the same commit and inputs are compared; the README
      section records which parts are identical (member list, modes, member checksums) and which
      differ (e.g. tar times, ordering, gzip header) and why, quoting the toolkit code responsible.
- [ ] Negative tests: a copy of an SPK with a removed `bin/ffmpeg`, a wrong `arch` and a leftover
      token each make the script fail (shown in the REPORT).
- [ ] shellcheck on the script → no findings.
- [ ] Gates: no product code changed; Go gates and script tests unaffected.

## Notes
- SPK-11 and SPK-13 run this script on the files they test or publish.
- Real defects found here become follow-up tasks (named in the REPORT), not silent fixes.
- Do not edit `DSM-NOTES.md`; toolkit findings belong in `toolkit/README.md`.
