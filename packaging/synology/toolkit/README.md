# Synology Package Toolkit in Docker

Walking skeleton of the SPK build (task SPK-04, ADR 0026 section 2, owner decision OD-SPK-2 B):
Synology's official Package Toolkit
[`pkgscripts-ng`](https://github.com/SynologyOpenSource/pkgscripts-ng) runs in a Linux
container with `CAP_SYS_ADMIN` under Docker Desktop and packs one SPK per architecture family
from **prebuilt** files. The toolkit compiles nothing; its chroot is only used to pack.

| File | Purpose |
|---|---|
| `Dockerfile` | toolkit host image (`debian:12-slim` + Python 3, git, xz/pixz, mount) |
| `run.sh` | container entrypoint: clone, deploy once, pack, copy the SPK out |
| `tarballs.sha256` | SHA-256 pins of the DSM 7.2 platform tarballs (see "Pinned tarballs") |
| `test.sh` | shellcheck, build-stage checks, pack and verify all families, caching |
| `testdata/demo/` | placeholder project files (fixture `build.env`, stub scripts, `conf/privilege`) |
| `testdata/stage-demo.sh` | stages the placeholder project for one family (stand-in for SPK-14) |
| `../package/SynoBuildConf/{depends,build,install}`, `../package/INFO.sh` | the toolkit side of the real package |

## Quick start (PowerShell, repository root)

```powershell
docker build -t spk-toolkit:dev packaging/synology/toolkit
$work = Join-Path $env:TEMP 'spk-demo'      # anywhere outside the repository

# Stage the placeholder project for a family (the real staging is spkstage, SPK-14) ...
docker run --rm --entrypoint bash -v "${PWD}:/src:ro" -v "${work}:/work" spk-toolkit:dev /src/packaging/synology/toolkit/testdata/stage-demo.sh x86_64 /work/project-x86_64
# ... and pack it. One command per family; replace x86_64 by armv8 or armv7 in both lines.
docker run --rm --cap-add SYS_ADMIN -v spk-toolkit:/toolkit -v "${work}/project-x86_64:/project:ro" -v "${work}/out:/output" spk-toolkit:dev x86_64
```

`--cap-add SYS_ADMIN` is the default and the only privilege the toolkit needs on Docker Desktop
(see "Privileges"); on Linux hosts with AppArmor see the fallback there.

The SPK lands in `$work/out/demo-x86_64-1.2.3-1040.spk`. The first run per family downloads and
deploys the platform environment (see "Caching"), later runs take about two seconds.
`stage-demo.sh` refuses a non-empty project directory; remove `$work` to start over.

Tests (Git Bash on Windows, or bash on Linux; Docker required):

```bash
bash packaging/synology/toolkit/test.sh          # everything, about a minute with cached environments
bash packaging/synology/toolkit/test.sh lint     # shellcheck only
bash packaging/synology/toolkit/test.sh x86_64   # lint, build-stage checks, one family
```

`test.sh` runs the toolkit container with `--cap-add SYS_ADMIN`; `SPK_TOOLKIT_PRIVILEGE`
replaces those options (for example `SPK_TOOLKIT_PRIVILEGE='--privileged'`).

## Interface of `run.sh` (stable for SPK-07 and SPK-14)

```
spk-toolkit-run <x86_64|armv8|armv7> [project-dir=/project] [output-dir=/output]
```

| Input | Meaning |
|---|---|
| family | selects the platform environment (table in "Platforms") |
| project dir | a staged toolkit project, mounted read-only; copied to `source/<ID>` in the volume |
| output dir | receives `<ID>-<family>-<DSM_VERSION>.spk` (name from `pkg_get_spk_family_name`); SPK-07 renames it |
| `TOOLKIT_PLATFORM` | optional: another platform of the family, for trying candidates |
| `TOOLKIT_ROOT` | optional: toolkit directory inside the container, default `/toolkit` (the volume) |

Exit status 0 on success, 1 on any failure (the reason is printed with `[spk-toolkit] failed:`,
followed by the last lines of the chroot's build and install logs), 2 on a usage error. Runs on
the same volume are serialised with a lock (`/toolkit/.lock`): `source/` and `result_spk/` are
shared.

### Staged project

The staged project that `SynoBuildConf/*` and `INFO.sh` expect:

| Path | Content | In the SPK |
|---|---|---|
| `build.env` | literal `sh` assignments, see below | not packed |
| `INFO.sh`, `SynoBuildConf/{depends,build,install}` | from `packaging/synology/package/` | – |
| `prebuilt/bin/<ID>`, `prebuilt/bin/ffmpeg`, `prebuilt/bin/ffprobe` | static ELF executables of the family | `package.tgz`: `bin/*`, 0755 |
| `prebuilt/share/licenses/**` | licence files and source notices (at least one file) | `package.tgz`: `share/licenses/**`, files 0644, directories 0755 |
| `scripts/`, `conf/` | package scripts and `privilege`/`resource` | `scripts/*` 0755, `conf/*` 0644 |
| `LICENSE`, `PACKAGE_ICON.PNG`, `PACKAGE_ICON_256.PNG` | optional; a note is logged when missing | 0644 |
| `WIZARD_UIFILES/` | optional | `*.sh` 0755, other files 0644 |

`build.env` fields (SPK-03 renders them; the fixture is `testdata/demo/build.env`):
`ID` (package name and binary name), `DISPLAY_NAME`, `DESCRIPTION`, `DESCRIPTION_GER`,
`MAINTAINER`, `MAINTAINER_URL`, `VERSION` (server version, not written to INFO), `DSM_VERSION`
(INFO `version`), `BETA` (`yes`/`no`), `OS_MIN_VER`, `ARCH` (INFO `arch`: the family,
optionally followed by further arch values, e.g. `armv7 armada38x monaco`; the first word is
the family the build and INFO checks use), `PORT` (INFO `adminport`). All are required by
`INFO.sh`.

**`build.env` grammar (contract for SPK-03/SPK-14).** `run.sh` sources `build.env` as root
outside the chroot, `INFO.sh` and `SynoBuildConf/*` source it inside, so `run.sh` checks the
file before anything sources it. Every line must be empty, a comment (`# ...`, optionally
indented) or one assignment of a single-quoted literal:

```
^([A-Z_][A-Z0-9_]*='[^']*'|[[:space:]]*(#.*)?)$      (POSIX extended regex, per line)
```

Assignment lines match exactly `^[A-Z_][A-Z0-9_]*='[^']*'$`, which is what SPK-03's generator
(`packaging/synology/internal/spkmeta`) emits. **Values contain no apostrophe**, not even
shell-escaped as `'\''` (orchestrator decision, one strict contract); nothing may follow the
closing quote (no trailing comment, no `export`, no `;`). Such a file cannot expand or run
anything. No CRLF.

**Value rules** (`INFO.sh`; INFO is sourced as shell by the toolkit and `pkg_dump_info` echoes
values unquoted, see "INFO keys"): no `"`, `'`, `\`, `$`, backquote, `*`, `?`, `[`, tab, newline
or double space in any value; `ID` matches `^[A-Za-z0-9][A-Za-z0-9._-]*$`; `DSM_VERSION`
`^[0-9]+(\.[0-9]+)*-[0-9]+$` (it becomes part of the SPK file name and path); `OS_MIN_VER`
`^[0-9]+\.[0-9]+-[0-9]+$`; `PORT` 1-65535 without leading zero; `BETA` `yes` or `no`; `ARCH`
starts with `x86_64`, `armv8` or `armv7`, and only `armv7` may be followed by `armada38x` and/or
`monaco` – the two DSM 7.2 platforms of the toolkit's list whose `plat_to_family` value is not
a family ("armv7 not ready", `pkg_util.sh:84-86`; ADR 0026 Notes). Other 32-bit ARM platforms
of `plat_to_family` (`armada370`, `armada375`, `armadaxp`, `comcerto2k`) are not DSM 7.2
platforms (`include/toolkit.config`).

## Pinned toolkit

`run.sh` clones `pkgscripts-ng` branch `DSM7.2` and checks out commit
`e4a8d54209a31cef21b7636ad58b1b0e86b5b201` ("Release for DSM7.2.2", 2025-05-28, the branch head
on 2026-10-31); the commit is pinned in one place, `PKGSCRIPTS_COMMIT` in `run.sh`. It verifies
that the commit is an ancestor of `origin/DSM7.2` and restores the commit when the clone has
local changes (`git diff --quiet HEAD`). The deploy marker records the toolkit commit; an
environment deployed by another commit is deployed again. The repository's default branch is now
`DSM7.4`; `DSM7.3` and `DSM7.4` exist but are out of scope (DSM 7.2 minimum, ADR 0026). The
platform environments are versioned separately on Synology's server: the 7.2 tarballs date
from 2024-06-06 and report DSM 7.2.2 build 72723 (`PkgVersion`, INFO `toolkit_version`).

### Pinned tarballs

`tarballs.sha256` (copied into the image) holds the SHA-256 of the seven DSM 7.2 tarballs
`run.sh` deploys: `base_env-7.2.txz` and `ds.<platform>-7.2.{dev,env}.txz` of `v1000`,
`rtd1619b` and `alpine`. Before every EnvDeploy `run.sh` hashes the three tarballs of the
platform (about 6 s) and stops if a hash differs or is missing; a mismatching file is deleted
with its download marker, so the next run downloads it again. Synology publishes no checksums:
the pins are **trust on first use** of the downloads of 2026-10-31 over HTTPS from
`global.synologydownload.com` (CloudFront). The CDN's `ETag` has the Apache form
`<size>-<mtime>` in hex (base: `46851fdc` = 1,183,129,564 bytes) and matched the downloaded
sizes; it proves size and date, not content. When Synology republishes a tarball, run.sh
refuses it until someone checks the new file and updates its line on purpose; trying another
platform (`TOOLKIT_PLATFORM`) needs its two lines first.

## Privileges

Measured on Docker Desktop 29.9.2, WSL2 kernel 6.18.33.2 (2026-10-31):

| `docker run` options | EnvDeploy | PkgCreate.py |
|---|---|---|
| none (Docker's default capabilities) | not tried separately (mounts `/proc` like PkgCreate.py) | **fails**: `mount: /toolkit/build_env/ds.v1000-7.2/proc: permission denied` |
| **`--cap-add SYS_ADMIN` (default)** | works for all three platforms (v1000, rtd1619b, alpine redeployed from the cache this way; armv8 also downloaded and deployed this way) | works for all three families; `test.sh` uses it |
| `--privileged` | works (first deployment of x86_64 and armv7) | works |

**Default: `--cap-add SYS_ADMIN`.** The toolkit `chroot`s into the platform environment
(`CAP_SYS_CHROOT`, in Docker's default set), mounts `/proc` into it before every chroot command
(`include/python/exec_env.py:66-68`, also during EnvDeploy) which needs `CAP_SYS_ADMIN`, and
EnvDeploy extracts 18,410 device nodes (`CAP_MKNOD`, default set; Docker's default device
rules allow creating any node, `c *:* m` and `b *:* m`, but not opening it). Least privilege (ADR 0026 section 2): unlike
`--privileged`, `--cap-add SYS_ADMIN` keeps Docker's device cgroup, seccomp profile and the
other dropped capabilities, so the container cannot open the host devices that Synology's
tarball recreates as nodes in the chroot.

**Fallback for Linux hosts with AppArmor (Ubuntu, GitHub-hosted runners), not verified here:**
Docker's default AppArmor profile denies `mount` even with `CAP_SYS_ADMIN`. Use
`--cap-add SYS_ADMIN --security-opt apparmor=unconfined` first, `--privileged` only if that
fails. Docker Desktop's VM runs no AppArmor, which is why the default works here. SPK-07 owns
the CI check.

### Why the toolkit never lives in a Windows bind mount

The toolkit, the tarballs and the environments live in the named volume `spk-toolkit` (the
Linux file system of Docker Desktop's VM). One deployed environment (v1000) holds 177,091
files, 3,969 symbolic links, 18,410 device nodes, 13 setuid/setgid files, 4,456 hard-linked
files and 292 paths that differ only in case (156 outside `/dev`, for example the kernel
headers `xt_dscp.h` and `xt_DSCP.h` in `/usr/include/linux/netfilter`, `/dev/fd0h1440` and
`/dev/fd0H1440`). NTFS behind a bind mount cannot store device nodes or Unix modes and owners
(every file appears as `0777 root`, see below), folds the case-colliding paths onto each other,
and the toolkit hard-links projects into the chroot (`cp -al`,
`include/python/link_project.py:26`), which needs one Linux file system for `source/` and
`build_env/`. A bind mount would also be slow for 7 GB of small files.

Only the staged project (read-only) and the output directory are bind mounts. Docker Desktop
shows every file of a Windows bind mount as `-rwxrwxrwx`, so modes of the staged project carry
no information on Windows: `SynoBuildConf/install` sets every mode explicitly, and the
"not executable" check of `SynoBuildConf/build` only bites on Linux hosts (CI) and in `test.sh`,
which runs it on a copy inside a container.

## Platforms

`arch` in INFO is the **family value from `build.env`** (`ARCH`), set explicitly by `INFO.sh`.
The toolkit writes no `arch` on its own: `pkg_dump_info` prints whatever `arch` variable
`INFO.sh` sets. `INFO.sh` cross-checks the first word of `ARCH` against the toolkit helper
`pkg_get_platform_family` (`include/pkg_util.sh:97`, mapping `plat_to_family` at line 67) and
fails if the environment belongs to another family, so binaries of one family are never packed
in another family's environment; `SynoBuildConf/build` checks the ELF machine of the prebuilt
files against the same family. The guide's examples use `pkg_get_unified_platform`, which
does not exist in the pinned `pkg_util.sh`.

| Candidate (ADR 0026 section 3) | `pkg_get_platform_family` | `pkg_get_spk_platform` | Deployed | Chosen for |
|---|---|---|---|---|
| `v1000` | `x86_64` | `v1000` | yes | **x86_64** |
| `geminilake` | `x86_64` | – | no | – |
| `rtd1619b` | `armv8` | – | yes | **armv8** |
| `armada37xx` | `armv8` | – | no | – |
| `alpine` | `armv7` | – | yes | **armv7** |
| `armada38x` | **`armada38x`** (listed as "armv7 not ready platforms", `pkg_util.sh:84-86`) | – | no | – (cannot produce `armv7`) |

Values of deployed platforms come from running the helpers inside their environment; the
others from `plat_to_family` called inside the v1000 environment (the same function the helper
uses), which avoids ~2.6 GiB of downloads for no new information. `v1000` (AMD Ryzen V1000)
was chosen over `geminilake` for being an AMD platform like the owner's device; any family
member produces the same family SPK (guide, "Prepare Environment"). `rtd1619b` was chosen over
`armada37xx` as the newer platform, `alpine` because `armada38x` does not map to `armv7`.

armv7 beyond `alpine`: the guide's mapping table lists only `alpine` and `alpine4k` under
`armv7`, and the toolkit's `armada38x` compiler flags use `-mfpu=vfpv3` without NEON. ADR 0026
Notes therefore decide `arch="armv7 armada38x monaco"` for the armv7 SPK (installation on
armada38x/monaco unconfirmed until a community test). `INFO.sh` accepts exactly that kind of
list (see "Value rules"); `test.sh` packs it in the alpine environment.

## Caching

The volume keeps the clone, the downloaded tarballs (`toolkit_tarballs/`) and the deployed
environments (`build_env/ds.<platform>-7.2/`). `run.sh` skips EnvDeploy when an environment is
deployed (marker in `/toolkit/.state` plus the environment's `PkgVersion`). EnvDeploy itself
always downloads the base tarball together with the platform tarballs and clears the
environment first (`EnvDeploy:65-69`, `pkgdeploy.py:191-194`), so `run.sh` downloads only the
missing tarballs with the toolkit's own `ToolkitDownloader` and then runs
`EnvDeploy -v 7.2 -p <platform> -D` (deploy from the cache, the guide's documented offline
mode). A download is recorded only after it completed, so an interrupted one is repeated.

Measured on 2026-10-31 (download speed of Synology's CDN varies):

| Family (platform) | First run: download | First run: EnvDeploy | Deployed size | Second run |
|---|---|---|---|---|
| x86_64 (v1000) | base 1.10 GiB + platform 0.91 GiB in 413 s | 22 s | 7.0 GB | 2.3 s, nothing downloaded |
| armv8 (rtd1619b) | platform 0.68 GiB in 53 s (base cached) | 26 s | 7.1 GB | 2.3 s, nothing downloaded |
| armv7 (alpine) | platform 0.71 GiB in 59 s (base cached) | 23 s | 6.6 GB | 2.2 s, nothing downloaded |

Wall time of the first runs: x86_64 440 s (its pack failed on a non-ASCII `depends`, see
"Pitfalls"; the corrected pack took 3 s), armv8 81 s, armv7 86 s. The clone takes a few
seconds. A redeploy from cached tarballs with `--cap-add SYS_ADMIN` (round 2, after the
SHA-256 check was added) took 30 s / 26 s / 24 s wall for x86_64 / armv8 / armv7: about 6 s
hashing, 16-21 s EnvDeploy, 0-2 s packing. The volume holds 24 GB with all three families (3.4 GiB tarballs). `test.sh` runs the
second run with `--network none` to prove that nothing is downloaded. Remove everything with
`docker volume rm spk-toolkit`; delete `toolkit_tarballs/` inside the volume to save 3.4 GiB
(a later re-deploy then downloads again).

## Toolkit facts (pinned commit)

- **No signing, no `--no-sign`.** The guide's option list (Toolkit > References) still names
  `--no-sign` and describes `-I` as "pack stage only", but `PkgCreate.py` at the pinned commit
  has neither a signing step nor that option: `--no-sign` is rejected
  (`PkgCreate.py: error: unrecognized arguments: --no-sign`, exit 2), `-I` means "Not install
  projects" (`PkgCreate.py:59`) and `-i` "Only install projects" (`:61`). Signing was removed
  with DSM 7.0 (guide release notes, "Remove package signing"); no file of the toolkit refers to
  `CodeSign` or `gpg`. "Signing disabled" therefore needs no flag; the SPKs carry no
  `syno_signature.asc` (asserted by `test.sh`).
- `run.sh` calls `PkgCreate.py -v 7.2 -p <platform> -c <ID>`: link, build, install, collect
  (`-c`, `PkgCreate.py:49`). `-v 7.2` overrides the `[default]` section of `depends`.
- The install stage runs **twice**, first with `--with-debug` (`NOSTRIP=NOSTRIP`,
  `PkgCreate.py:161-167`); that pass would produce `*_debug.spk`. `SynoBuildConf/install` skips
  it, because prebuilt binaries have no debug variant.
- `SynoBuildConf/build` and `install` are **sourced by bash in a subshell** (`include/build:441`,
  `include/install:158`); their exit status is ignored. Success is decided by searching the logs
  (`include/errors.py:16-30`): a line containing `Error`, `error:`, `fatal error`,
  `No such file or directory` (install) and a few more fails the stage. Our scripts print
  `Error: <reason>` and must never print those words otherwise.
- Helpers in `include/pkg_util.sh`: `pkg_dump_info` (`:180`), `pkg_make_package` (`:209`),
  `pkg_make_spk` (`:280`), `pkg_get_spk_family_name` (`:276`), `pkg_get_platform_family`
  (`:97`), `pkg_get_spk_platform` (`:103`).
- `pkg_make_package` writes `package.tgz` with `tar cJf` (`:215`): despite its name the inner
  archive is **xz-compressed**. It records the payload size for INFO `extractsize`.
- `pkg_make_spk` appends `extractsize`, `toolkit_version` and `create_time` to INFO (`:307-311`)
  and packs the SPK with `tar cf ... $(ls)` (`:317`): an uncompressed tar, members in `ls`
  order, owner `root/root`, current file times (not reproducible byte for byte).
- **Tar flavour unconfirmed**: only the commands are known (`tar cf` for the SPK, `tar cJf`
  for `package.tgz`, run inside the chroot, whose `tar` is GNU tar 1.32 with the compiled-in
  default `--format=gnu` per `tar --show-defaults`). The header format of the produced files has
  not been read from the headers, and whether DSM requires a particular format is not
  documented; SPK-10 records it from the headers.
- The collect stage copies `/image/packages/<package>*<version>*spk` of the chroot to
  `/toolkit/result_spk/<package>-<version>/` (`include/python/pkguniform.py:289-312`);
  `run.sh` takes it from there.
- Inside the chroot: bash 4.3.33, Python 3.4.3 and 2.7.9, `POSIX` locale, time zone
  `Asia/Taipei` (so INFO `create_time` is Taipei time).

### INFO keys

`INFO.sh` sets `package`, `version`, `displayname`, `description`, `description_ger`,
`maintainer`, `maintainer_url`, `os_min_ver`, `arch`, `adminport`, `adminprotocol=http` and,
only when `BETA=yes`, `beta="yes"` (a release writes no `beta` key: `pkg_dump_info` skips empty
variables), and prints them with `pkg_dump_info`. **No `adminurl`**: DSM opens
`adminprotocol://<host>:adminport/adminurl`, so `/` would give `...:8460//` (ADR 0026 Notes,
"adminurl is omitted"; `test.sh` asserts the line is absent). Its key list (`pkg_util.sh:182-188` plus
`description_<lang>`/`displayname_<lang>` for 20 DSM languages) covers every key of the guide's
INFO pages **except** `support_url`, `support_move`, `install_on_cold_storage` and
`extractsize`. `INFO.sh` appends `support_move="no"` itself (ADR 0026 section 10);
`extractsize` comes from `pkg_make_spk`. Example (x86_64 fixture):

```
package="demo"
version="1.2.3-1040"
maintainer="Example Maintainer"
maintainer_url="https://example.invalid/demo"
arch="x86_64"
adminprotocol="http"
adminport="9999"
os_min_ver="7.2-64570"
beta="yes"
description="Placeholder package of the toolkit walking skeleton."
displayname="Demo Server"
description_ger="Platzhalterpaket für den Toolkit-Durchstich."
support_move="no"
extractsize="40"
toolkit_version="72723"
create_time="20261031-20:54:28"
```

`pkg_dump_info` prints `key="value"` with an **unquoted** `echo` (`pkg_util.sh:193`): a `"`,
`\`, `$` or backquote breaks the INFO syntax, `*`, `?` and `[` may expand against files of the
working directory, and runs of spaces collapse. `INFO.sh` rejects such values (and
apostrophes, tabs and newlines) with an error; `test.sh` has one case per character, and
`run.sh` already refuses an apostrophe in `build.env` (both `'It's'` and `'It'\''s'`). UTF-8
text such as the umlaut above passes through unchanged.

## Pitfalls

- `SynoBuildConf/depends` is read by Python 3.4 inside the chroot with an ASCII locale. One
  non-ASCII byte (a `section` sign in a comment did it) makes the dependency list empty; the
  build and install stages then report "0 projects, 0 failed" and only the collect stage fails
  with `ConfigNotFoundError: .../source/<ID>/INFO`. `run.sh` refuses non-ASCII bytes in
  `SynoBuildConf/` and CRLF line endings in the project scripts before calling the toolkit.
  `build.env` and therefore INFO values may contain UTF-8.
- Docker Desktop shows bind-mounted files as `0777`, see "Why the toolkit never lives in a
  Windows bind mount".
- `install` copies with `install`/`cp` and checks every call (`|| spk_install_fail`): an error
  such as "Permission denied" or "No space left on device" matches none of the toolkit's log
  patterns and would otherwise pass unnoticed. Its copy loops read from process substitutions,
  not pipes, so a failure inside a loop leaves the script.
- Tests that start many short containers in a row must not share a fixed scratch path in the
  volume: in round 2 the clean-up of one `docker run` raced the next one on
  `/tmp/info-test` inside the chroot (directory vanished mid-run). `test.sh` now uses
  `mktemp -d` per call.
- Git on Windows does not record the executable bit of new files (`core.filemode=false`).
  Nothing relies on it: the image sets the mode of `run.sh` itself (`Dockerfile`), `install`
  runs `INFO.sh` through `bash`, the toolkit sources `SynoBuildConf/*`, `test.sh` calls
  `stage-demo.sh` through `bash`, and `install` sets the modes of everything it packs.
