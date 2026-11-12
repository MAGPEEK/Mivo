# Emulated install and playback smoke (SPK-11)

`smoke.sh` installs one server SPK into a DSM-like layout inside a `debian:stable-slim`
container of the family's platform, starts it through the package's own `start-stop-status`,
plays the generated test corpus through it with `scripts/playback-smoke` (both reference
profiles), stops it and runs the upgrade and uninstall paths. armv8 and armv7 run under QEMU
user-mode emulation, so the packages of the families the owner has no hardware for are
exercised before a release (OD-SPK-4: armv7 is published only if this passes, labelled "tested
under emulation only").

QEMU proves the instruction set and the behaviour of the binaries and scripts, not DSM's kernel
or real performance (ADR 0026 consequences). Do not draw performance conclusions from the
timings.

## Run

The Docker host must be **x86_64 (amd64)**: `kernelsim`, `sysprobe` and `qemu-x86_64` are
x86_64 binaries that run natively in every container, and the x86_64 family runs without
emulation. `smoke.sh` checks `docker info --format '{{.Architecture}}'` and refuses anything
else. Git Bash on Windows (Docker Desktop brings the binfmt QEMU for arm64 and arm/v7) or bash
on an x86_64 Linux host (install binfmt QEMU, for example
`docker run --privileged --rm tonistiigi/binfmt --install arm64,arm`).
From the repository root, after building the SPKs (`packaging/synology/build.sh`, see
`../../README.md`; they are named `<id>-<version>-<family>.spk`) and generating the corpus:

```bash
go run ./scripts/gen-testmedia      # once: testdata/media and its manifest
bash packaging/synology/test/smoke/smoke.sh x86_64 packaging/synology/dist/*-x86_64.spk   # linux/amd64, native
bash packaging/synology/test/smoke/smoke.sh armv8  packaging/synology/dist/*-armv8.spk    # linux/arm64, QEMU cortex-a53
bash packaging/synology/test/smoke/smoke.sh armv7  packaging/synology/dist/*-armv7.spk    # linux/arm/v7, QEMU cortex-a9, cortex-a15 and the worst case
```

The globs assume one SPK per family in `dist/` (what one build leaves there).

One command per family; the exit status is 0 only when every step passed. The last lines are a
summary with one `PASS`, `FAIL` or `INFO` line per step and `SUMMARY <family>: N passed, M
failed`. The `INFO` line per CPU model holds the FFmpeg version line, the SPK size and the
duration of the whole run; the playback lines hold the scan duration and the run summary of
each profile.

```
smoke.sh [options] <family> <file.spk> [cpu-model ...]
```

| Option | Meaning |
|---|---|
| `cpu-model ...` | QEMU CPU models, one complete run each. Defaults: armv8 `cortex-a53`; armv7 `cortex-a9`, `cortex-a15` and `cortex-a9,neon=off,vfp-d32=off` (STiH412 versus Alpine versus the Armada 38x worst case of ADR 0026 Notes: VFPv3-D16 without NEON; a VFPv4, NEON or D32 dependency shows up as `SIGILL`). Other QEMU models and properties can be given instead. x86_64 takes none (see below). |
| `--host-port N` | host port `127.0.0.1:N` that publishes the package's `adminport` (default 8460) |
| `--share NAME` | name of the shared folder the corpus is mounted as (default `video`) |
| `--case-timeout D` | `-case-timeout` of the first playback run (default: playback-smoke's 2m) |
| `--slow-case-timeout D` | also `SMOKE_SLOW_CASE_TIMEOUT`; when every failure of a profile run is a timeout, the profile runs again with this `-case-timeout` and `-scan-timeout` (default 15m), and both runs are reported (the first run's log stays as `playback-<profile>-timeouts.log`). A failure counts as a timeout when it reports `deadline exceeded` or when its case ran at least the case timeout (a timeout that cuts a download or the probe short surfaces as another error). Any other failure, and any failure of the rerun, is a `FAIL`. |
| `--ffprobe PATH` | the local ffprobe playback-smoke probes the downloads with (default `ffprobe` on PATH) |
| `--work DIR` | also `SMOKE_WORK`; directory for the logs (default: a new temporary directory). It is kept: `inspect.log`, and per CPU model `dsm.log` (every step, script output), `playback-<profile>.log`, `logs/` (package log and server log) and `var-log-packages/`. |
| `--keep` | leave the container running for a look inside (`docker exec -it spk-smoke-<family>-<cpu> bash`). Without it the container of each CPU model is removed after its run, and also when the script exits early or is interrupted (EXIT, INT and TERM traps). |

Needs bash 4 or newer, Docker, Go (builds `scripts/playback-smoke` once per run), curl and
ffprobe. On the Windows dev machine (Docker Desktop) a run took about 25 s for x86_64 and about
1.5 minutes per emulated CPU model, most of it the transcode cases (plus the image build under
QEMU the first time).

## What one run does

1. `../inspect/inspect-spk.sh --release` on the SPK (SPK-10, image `spk-inspect:dev`); nothing
   is installed unless it passes.
2. Builds `kernelsim`, `sysprobe` and a static `qemu-x86_64` from
   `../../ffmpeg/build/verify/Dockerfile` (as `verify.sh` does), the image of `Dockerfile` here
   for the family's platform (`spk-smoke:<family>`; `debian:stable-slim` plus `xz-utils`,
   because `package.tgz` is xz-compressed) and `scripts/playback-smoke`.
3. Per CPU model a container with the SPK, the corpus `testdata/media` mounted **read-only** as
   `/volume1/<share>`, the tools and this directory; port `adminport` (8460) published on the
   host. `dsm.sh` then does, as root, what Package Center does, and runs every package script
   **as the package user** (`setpriv`) **under `kernelsim`** with the family's syscall list
   (`syscalls-after-3.10.txt` for x86_64 and armv7, `syscalls-after-4.4.txt` for armv8): the
   script, the server and every FFmpeg process the server starts get `ENOSYS` for each syscall
   the oldest DSM 7.2 kernel of the family lacks. Under QEMU, `QEMU_UNAME` makes `uname` report
   that kernel (3.10.108 or 4.4.302) and `QEMU_CPU` selects the CPU; both are passed on to every
   package process, so FFmpeg runs on the same CPU model as the server.
4. Steps (DSM-NOTES.md section 5 "Execution order"):
   - share snapshot; **install**: unpack the SPK, package user `<id>`, `/var/packages/<id>/`
     `target`, `var`, `tmp`, `home` as links to `/volume1/@appstore|@appdata|@apptemp|@apphome/<id>`
     (owner `<id>`, `home` 0700, others 0755), `preinst` from the unpacked SPK (no wizard
     keys: the install wizard asks nothing since SPK-17), `package.tgz` into `target`, scripts
     and `conf` into `/var/packages/<id>/`, `postinst` (no `config.yaml`), then the resources
     (`port-config` copies the `.sc` to `/usr/local/etc/service.d/`; there is no `data-share`);
   - **probe**: `uname` and `AT_HWCAP` as the package user, `sysprobe` proving the filter;
   - **versions**: `bin/<id> --version`, `bin/ffmpeg -version`, `bin/ffprobe -version`;
   - `prestart`, **`start`** (`SYNOPKG_PKG_STATUS=INSTALL`), **`status`** exits 0, the API
     answers on the published port;
   - **sign-in** (on since M7-25): the server starts in setup state, so the script runs the
     first-run setup (`POST /api/v1/setup`, client `cli`, LAN peer) for the throwaway admin
     `smoke` with a random password generated per run, and keeps the bearer token for every
     later `curl`; after a restart on a kept data directory it signs in instead
     (`POST /api/v1/auth/session`). The password lives in the script's memory and reaches
     `playback-smoke` only through `MIVO_SMOKE_PASSWORD` (`-user smoke -password-env`); it and
     the token are never printed or written to disk. Only `system/info` and the setup state are
     called without a token. A failed sign-in is reported with the HTTP status and problem code
     only. SPKs of pre-M7 servers (no accounts, `GET /api/v1/setup` answers 404) are not
     supported by this smoke. Do not run the script under `bash -x`;
   - **first start state**: `GET /api/v1/libraries` is `{"libraries":[]}`, `GET
     /api/v1/folder-roots` lists `/volume1` and `installKind` `synology` (from the
     `start-stop-status` environment); then the corpus library is created through `POST
     /api/v1/libraries` with that root and the share name, as the web app does;
   - **playback-smoke** with `chromium-desktop` and `firefox-desktop`
     (`-manifest testdata/media/manifest.json`), each scanning the library first;
   - `prestop`, **`stop`** exits 0 within 20 s; then `status` exits 3, no server or FFmpeg
     process is left, `tmp/transcode/sessions` exists (the playback ran transcodes) and is empty,
     the share is unchanged and still read-only, the server log `var/<id>.log` exists and has no
     `ERROR` line;
   - **upgrade** to the same SPK (there is no older one): old `prestop`/`stop`, new
     `preupgrade`, old `preuninst` and `postuninst` (with the old `SYNOPKG_PKGVER`), `target` and
     `tmp` replaced, new `preinst`, `postinst`, `postupgrade` (`UPGRADE`, `SYNOPKG_OLD_PKGVER`
     set), no `config.yaml` appears and `var/data` is kept; `prestart`/`start`, `status` 0, and the
     scan the server starts on its own finds every file **unchanged** (none added): the database
     was kept (a scan state `failed` ends the wait as a `FAIL`);
   - **uninstall keeping the data** (`wizard_delete_data=false`): `var` and `home` keep
     `data/`; **reinstall**: the start finds the catalogue and the library again; stop and the
     checks after the stop;
   - **uninstall deleting the data** (`wizard_delete_data=true`): `var` and `home` are empty, the
     directories and the share stay;
   - **second scenario, a fresh container: upgrade from an alpha.2-shaped installation**: after
     an install, `dsm.sh seed-alpha2 <share>` writes the `config.yaml` that 0.1.0-alpha.2's
     `postinst` wrote (`path: "/var/packages/<id>/shares/<share>"`) and the link
     `shares/<share>`; the upgrade then runs the new `preupgrade` (records the link target),
     drops the link as DSM does without a `data-share` resource, and `postupgrade` rewrites the
     path to `/volume1/<share>` and saves `config.yaml.bak-0.1.0-alpha.2`; after the start,
     `GET /api/v1/libraries` lists the library with the path `/volume1/<share>`. It is the new
     SPK on both sides (no alpha.2 SPK is at hand), so alpha.2's own scripts are not run; SPK-12
     upgrades the real alpha.2 on the owner's NAS.

Script output is appended to `/var/log/packages/<id>.log` as on DSM; the dialog text a script
writes to `$SYNOPKG_TEMP_LOGFILE` is shown as `dialog:` lines.

## Deviations from DSM and why

### QEMU and the process check

`start-stop-status` recognises its server by `[ /proc/<pid>/exe -ef $PKG_BINARY ]`. Under QEMU
user mode the kernel's `/proc/<pid>/exe` of an emulated process names the interpreter
(`/usr/bin/qemu-aarch64`, `/usr/bin/qemu-arm`), never the guest binary; QEMU emulates the link
only for a process's own PID. Evidence (Docker Desktop 29.9.2, kernel 6.18):

```
$ docker run --rm --platform linux/arm/v7 debian:stable-slim sh -c 'sleep 5 & p=$!; sleep 0.3; readlink /proc/$p/exe; [ /proc/$p/exe -ef /usr/bin/sleep ] && echo SAME || echo DIFF; tr "\0" "|" </proc/$p/cmdline'
/usr/bin/qemu-arm
DIFF
/usr/bin/qemu-arm|/usr/bin/sleep|sleep|5|
```

Unchanged, every emulated `start` would end with "did not start within 5 s" while the server
runs. For emulated families `dsm.sh` therefore appends `qemu-shim.sh` to the **installed copy**
of `scripts/common.sh` (never to the SPK). `start-stop-status` defines `is_server_process`
before it sources `common.sh`, so the shim's definition wins: it runs the original test first
and otherwise accepts a live process whose `exe` is a QEMU interpreter and whose guest binary
(argv[1] of the interpreter, binfmt's preserve-argv0 layout) is the package binary. Everything
else - PID file, health check, `SIGTERM`, the 20 s stop budget, `SIGKILL` - is the package's
own code. The x86_64 run uses the scripts unchanged, so the original check is proven there.

### x86_64 runs natively

There is no binfmt handler for x86_64 on an x86_64 host, and running the server through an
explicit `qemu-x86_64` would need a wrapper in place of the package binary. The x86_64 run
therefore executes the package unchanged, natively, under `kernelsim` with the 3.10 list, but
`uname` of the server reports the Docker host's kernel. This is the stricter case for the Go
runtime: it attempts the newer syscalls (`copy_file_range`, `pidfd_open`, `clone3`, ...) and
must cope with `ENOSYS`, where a 3.10 `uname` would make it skip some of them. The version
commands of the x86_64 run (`bin/<id> --version`, `ffmpeg -version`, `ffprobe -version`) run
through `qemu-x86_64 -cpu Opteron_G1 -r 3.10.108`, so they see the DSM kernel and a baseline CPU;
the playback itself runs on the host CPU. FFmpeg on baseline x86_64 CPUs is covered by
`../../ffmpeg/build/verify/verify.sh` (SPK-15).

### Other limits

- `kernelsim` skips syscall names its libseccomp does not know (newer than its table, for
  example `statmount`, `mseal`); they stay allowed and the probe step lists them.
- The package scripts themselves also run under the syscall filter, and Debian's `dash`,
  coreutils and `setpriv` stand in for DSM's shell and tools (DSM's `/bin/sh` is unconfirmed,
  DSM-NOTES.md section 5; the script tests in `../scripts` cover BusyBox, bash and dash).
- The resource workers are modelled, not run: the package user's read access to the share is
  the read-only mount (on DSM the user grants it in Control Panel), and the firewall is not
  involved. Boot (`prestart`/`start` with an empty status), the wizards themselves and Package
  Center's dialogs are not exercised; SPK-12 checks them on the owner's NAS.
- The upgrade runs from an SPK to itself; a version-to-version upgrade on DSM is part of SPK-12.

## Checks of the harness

```bash
MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd -W):/mnt:ro" -w /mnt koalaman/shellcheck:v0.11.0 packaging/synology/test/smoke/smoke.sh packaging/synology/test/smoke/dsm.sh packaging/synology/test/smoke/qemu-shim.sh
docker run --rm -i hadolint/hadolint < packaging/synology/test/smoke/Dockerfile
```

On Linux drop `MSYS_NO_PATHCONV=1` and use `$(pwd)`.
