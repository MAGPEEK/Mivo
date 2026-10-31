# SPK-10 Verify the toolkit output
Owner: mivo-synology
Depends on: SPK-07
Files/areas: `packaging/synology/test/inspect/**` (new: inspection script + expected-member list),
`packaging/synology/toolkit/README.md` (new section "Output verification")

## Goal
Every server SPK the toolkit produces is checked against ADR 0026 and its Notes – structure,
modes, INFO, FFmpeg status, no leftovers from the chroot – and the reproducibility limits of the
toolkit's packing are known, so releases ship exactly what was tested.

## Acceptance criteria
- [ ] `packaging/synology/test/inspect/inspect-spk.sh [--release] <file.spk> <family>` (POSIX `sh`,
      runs in a `debian:stable-slim` container) checks and reports each item, exit 1 on any failure:
      - outer archive is an uncompressed tar; members equal the expected list (`INFO`,
        `package.tgz`, the seven scripts plus `common.sh`/`package.env`, `conf/privilege`,
        `conf/resource`, both icons with 64 × 64 and 256 × 256 pixels, `LICENSE`,
        `WIZARD_UIFILES/*`); no signature member;
      - INFO: `package`, `displayname`, `version` matching `^[0-9]+\.[0-9]+\.[0-9]+-[0-9]+$` with
        every number ≤ 2^31−1, `arch` exactly as ADR 0026 Notes (`x86_64`, `armv8`,
        `armv7 armada38x monaco`), `os_min_ver="7.2-64570"`, `beta="yes"` for pre-releases,
        `adminport="8460"`, `adminprotocol="http"`, **no `adminurl`**, `description` and
        `description_ger`, `support_move="no"`, `extractsize`, `toolkit_version`; no `thirdparty`,
        `startable`, `changelog`;
      - `package.tgz` (xz-compressed): `bin/<id>`, `bin/ffmpeg`, `bin/ffprobe` executable, statically
        linked, ELF machine of the family, ELF ABI note ≤ the family's oldest DSM 7.2 kernel
        (x86_64/armv7 3.10.108, armv8 4.4.302); `share/licenses/ffmpeg/SOURCE.txt` present;
        `port_conf/<id>.sc` with section, `title`, `desc`, `port_forward="no"`, `dst.ports`;
        nothing from the build environment (no `SynoBuildConf`, `build.env`, `prebuilt/`, absolute
        paths, `..`, debug files);
      - `scripts/*` executable with LF endings; `conf/privilege` without root; every JSON member
        parses; no unreplaced `@NAME@` token anywhere;
      - `SOURCE.txt` status `interim` → warning; with `--release` → failure.
- [ ] The script passes on all three SPKs of SPK-07 (interim FFmpeg → warnings only, and `--release`
      fails as expected); output in the REPORT. After SPK-16 the orchestrator re-runs it with
      `--release`.
- [ ] Tar flavour (unconfirmed so far, ADR 0026 Notes): the README section records the format of the
      outer SPK and of the tar inside `package.tgz` (GNU, ustar or pax), determined from the header
      magic (`ustar  ` + NUL = GNU, `ustar` + NUL + `00` = POSIX) or `file`, and whether any member
      needs GNU or pax extensions (long names, large files); quote the toolkit's `tar` version in
      the chroot.
- [ ] Reproducibility: two SPK-07 builds of the same commit and inputs are compared; the README
      section records what is identical (member list, modes, member checksums) and what differs
      (INFO `create_time`, tar times and order, xz stream) and why, quoting the toolkit code
      (`pkg_util.sh` line numbers).
- [ ] Negative tests: copies of an SPK with a removed `bin/ffmpeg`, a wrong `arch`, an `adminurl`
      line, a leftover token and a dynamically linked `ffmpeg` each make the script fail (REPORT).
- [ ] shellcheck on the script → no findings.
- [ ] Gates: no product code changed; Go gates and script tests unaffected.

## Notes
- SPK-11, SPK-12 and SPK-13 run this script on the files they test or publish.
- Real defects found here become follow-up tasks (named in the REPORT), not silent fixes.
- Do not edit `DSM-NOTES.md`; toolkit findings belong in `toolkit/README.md`.
