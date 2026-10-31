# SPK-04 Package Toolkit in Docker: walking skeleton
Owner: mivo-synology
Depends on: none
Files/areas: `packaging/synology/toolkit/**` (new: container definition, run script,
`README.md`, placeholder test project under `toolkit/testdata/`),
`packaging/synology/package/SynoBuildConf/{depends,build,install}` (new),
`packaging/synology/package/INFO.sh` (new)

## Goal
Synology's official Package Toolkit (`pkgscripts-ng`, branch `DSM7.2`) runs on the Windows dev
machine inside Docker Desktop and packs one SPK per family from prebuilt files, proving the
build approach of ADR 0026 §2 (OD-SPK-2 B) before anything else depends on it.

## Acceptance criteria
- [ ] A toolkit container definition (Debian or Ubuntu with what `EnvDeploy`/`PkgCreate.py` need)
      and a run script: it clones `pkgscripts-ng` at a **pinned commit** of branch `DSM7.2` into a
      **named Docker volume** (e.g. `spk-toolkit`), runs `EnvDeploy -v 7.2 -p <platform>` only if
      the environment is not deployed yet, copies a project into `source/<project>/`, runs
      `PkgCreate.py` for DSM 7.2 with **signing disabled**, and copies the resulting SPK to an
      output directory. One documented `docker run` command per family works from PowerShell.
- [ ] Privileges: the README records which `docker run` options are required (expected
      `--privileged`; try a narrower `--cap-add` set and record the result) and why (chroot,
      mounts). Toolkit and environments never live in a Windows bind mount (README explains why).
- [ ] Platforms: one representative DSM 7.2 platform per family is deployed (candidates in ADR 0026
      §3). The README lists, for every candidate tried, the `arch` value the toolkit writes into
      INFO, and names the chosen platform per family. Every produced SPK has the **family** value
      in INFO `arch` (`x86_64`, `armv8`, `armv7`) – from the toolkit helper or set explicitly by
      `INFO.sh` (README says which).
- [ ] `SynoBuildConf/build` compiles nothing and fails with a clear message if the prebuilt files
      of the family are missing or not executable; `SynoBuildConf/install` copies the prebuilt
      files into the package layout (`bin/` 0755, `share/licenses/` 0644) and packs with the
      toolkit's helpers; `SynoBuildConf/depends` is minimal for DSM 7.2. `INFO.sh` sources
      `build.env` (brand values, version, DSM version, beta flag, family) and writes INFO with the
      toolkit's helper, appending keys the helper does not support (README lists them).
- [ ] The placeholder test project (fixture `build.env` with a dummy brand such as `ID=demo`, stub
      scripts, a tiny static executable as "server" and stubs for `ffmpeg`/`ffprobe`) packs into
      three SPKs; `tar -tvf` of each shows `INFO`, `package.tgz`, `scripts/`, `conf/` and no
      signature member; INFO contains the fixture's `package`, `version` and the family `arch`.
- [ ] Caching: a second run for the same family downloads nothing; the README records download
      size, deployed size and duration of the first and second run per family.
- [ ] Brand-free: `rg -i mivo packaging/synology/package packaging/synology/toolkit` finds nothing
      (the project name comes from `build.env`).
- [ ] shellcheck on the run script, `SynoBuildConf/*` and `INFO.sh` (`-s sh` or `-s bash` as the
      toolkit runs them) → no findings.
- [ ] Gates: no Go code; shellcheck clean; the commands above succeed on Docker Desktop.

## Notes
- If the toolkit cannot run in Docker Desktop at all, stop with status BLOCKED and the evidence;
  the owner then chooses a Linux VM/WSL distribution as build host or revisits OD-SPK-2.
- Names of helpers and options (`pkg_make_package`, `pkg_make_spk`, `pkg_dump_info`, the
  no-sign option, `-c`/`-I` modes) come from the pinned sources; quote file and line in the README.
- The real server binary, FFmpeg and staging arrive with SPK-14 and SPK-07; keep the run script's
  interface (family, project dir, output dir) stable for them.
- Do not edit `DSM-NOTES.md` (SPK-01 owns it in this wave) or ADR 0026; findings go to
  `toolkit/README.md` and the REPORT.
- Network: the clone and the platform environments are approved downloads.
