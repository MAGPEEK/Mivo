# SPK-13 Attach the SPKs to the GitHub Release
Owner: mivo-release
Depends on: SPK-16, SPK-12 PASS, SPK-11 PASS per published family, M2-36 PASS + APPROVE, the release
decision for 0.1.0-alpha.2 (OD-9) and the existing tag/GitHub Release `v0.1.0-alpha.2`
Files/areas: `docs/reports/release-0.1.0-alpha.2.md` (append a "Synology packages" section)

## Goal
Synology users can download the media-server SPK for their family, verify it and install it from the
0.1.0-alpha.2 GitHub Release, with FFmpeg's corresponding source next to it.

## Acceptance criteria
- [ ] The uploaded SPKs are **the files that were tested**: built with SPK-07's command from a clean
      checkout (`VERSION=0.1.0-alpha.2`, `REVISION=<commit>`, `PACKAGE_REVISION=0`) of the commit
      the tag points to, inspected by SPK-10's script, run through SPK-11 and, for x86_64, SPK-12
      (same SHA-256 as recorded there). The toolkit's packing is not byte-reproducible, so a
      rebuild means re-running `inspect-spk.sh` and SPK-11 for every family and steps 3–8 of
      SPK-12 for x86_64 before upload.
- [ ] **Interim refusal**: the task stops with FAIL if
      `grep -q '^[^#][^ ]* interim ' packaging/synology/ffmpeg/sources.lock` succeeds at the release
      commit, or if `inspect-spk.sh --release` fails on any SPK (a packed `SOURCE.txt` with status
      `interim`). No release ever contains an interim FFmpeg build (ADR 0026 Notes).
- [ ] Uploaded to the GitHub Release `v0.1.0-alpha.2`: the SPK of every family allowed by OD-SPK-4,
      `SHA256SUMS` covering them, and the FFmpeg corresponding source archive of SPK-15 (FFmpeg and
      every linked library, OD-SPK-3) with its checksum.
      `gh release view v0.1.0-alpha.2 --json assets` lists them; a downloaded SPK matches
      `SHA256SUMS`.
- [ ] Release notes gain a "Synology (DSM 7.2+)" section: which file for which family, manual
      install link to `packaging/synology/README.md`, the version shown by Package Center
      (`0.1.0-1020`), LAN-only note, armv7/armv8 test status ("tested under emulation only" where
      applicable).
- [ ] `docs/reports/release-0.1.0-alpha.2.md` records asset names, sizes, checksums, commands and
      the approval that covered the upload.
- [ ] Gates: no code changes.

## Notes
- Approval: as every release (CLAUDE.md step 6) together with the owner's standing GitHub
  instruction; no Docker push is part of this task.
- Never upload SPKs built from a dirty tree or a commit other than the tag.
- If OD-SPK-1 A applies (Docker released first), this task runs later against the same release;
  a packaging-only fix uses `PACKAGE_REVISION=1` and replaces the assets, noted in the release
  notes.
