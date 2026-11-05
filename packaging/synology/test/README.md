# Package script tests

Tests of the DSM package files in `../package/`: the service control script of SPK-05
(`scripts/start-stop-status`, `scripts/common.sh`; `scripts/package.env` exists only in the
staged package, written by `spkstage` from `spkmeta`), the lifecycle scripts
of SPK-06 (`scripts/preinst`, `postinst`, `preuninst`, `postuninst`, `preupgrade`,
`postupgrade`) and the SPK-06 templates (`WIZARD_UIFILES/*`, `conf/privilege`, `conf/resource`,
`port/service.sc`). They run with [bats-core](https://github.com/bats-core/bats-core) in Docker
against a fake server, so they need neither DSM nor the real server binary.

## Run

From the repository root; Docker Desktop is enough, nothing else is installed. The test image
(`Dockerfile` here) is `bats/bats:1.14.0`, pinned by digest, plus dash and jq in pinned
Alpine versions. The tests run as root in the throw-away container and write the link
`/var/packages/demo` of the container (see `scripts/helpers.bash`); run them in the image, not
on a host.

```powershell
# PowerShell
docker build -t spk-script-test:dev packaging/synology/test
docker run --rm -v "${PWD}:/code" -w /code spk-script-test:dev packaging/synology/test/scripts
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/package/scripts/start-stop-status packaging/synology/package/scripts/common.sh packaging/synology/package/scripts/ffmpeg-capabilities.sh packaging/synology/package/scripts/preinst packaging/synology/package/scripts/postinst packaging/synology/package/scripts/preuninst packaging/synology/package/scripts/postuninst packaging/synology/package/scripts/preupgrade packaging/synology/package/scripts/postupgrade
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 packaging/synology/test/scripts/start-stop-status.bats packaging/synology/test/scripts/lifecycle.bats packaging/synology/test/scripts/ffmpeg-capabilities.bats packaging/synology/test/scripts/package-files.bats packaging/synology/test/scripts/helpers.bash packaging/synology/test/fake-server.bash packaging/synology/test/fake-ffmpeg.sh packaging/synology/test/fixtures/package.env
```

In Git Bash, prefix each command with `MSYS_NO_PATHCONV=1` and use `"$(pwd -W)"` instead of
`"${PWD}"`, so that `/code` and `/mnt` are not rewritten into Windows paths. On Linux and in CI
the PowerShell lines work unchanged in bash.

The bats run takes a few minutes: every case runs under four shells, and a few cases wait for
the shortened start and stop budgets or the 5 s exec wait. `--filter 'stop.*busybox'` (a
regular expression over the test names) runs a subset.

## What runs where

- `scripts/*.bats` – the test files; bats runs every `.bats` file of the directory.
  `start-stop-status.bats` covers SPK-05 and the SPK-06 hardening of `start-stop-status`,
  `lifecycle.bats` the lifecycle scripts and the `common.sh` helpers they use (browse roots, the alpha.2 upgrade rewrite (SPK-17),
  YAML quoting, the package-directory guard of every deletion), `package-files.bats` the static
  templates (JSON syntax, privilege, resource, service file, the info-only install wizard and
  no media folder question), `ffmpeg-capabilities.bats` the FFmpeg capability check
  `scripts/ffmpeg-capabilities.sh` of task M3-16j (required disc demuxers and protocols and photo
  decoders, the reported HEIC/AVIF decoders, the timeout(1) limit of every ffmpeg call; task
  M3-16j-b) that postinst runs into the package log.
- `scripts/helpers.bash` – shared setup: a DSM-like package tree per test in `$BATS_TEST_TMPDIR`
  (`target`, `var`, `tmp` and `home` are links into `volume1/@appstore|@appdata|@apptemp|@apphome/<id>`
  as on DSM; `scripts/` with every package script and the fixture `package.env`), reached
  through the link `/var/packages/<id>` of the container, so the `SYNOPKG_*` variables have
  DSM's real shape `/var/packages/<id>/var` (`postuninst` deletes only below directories of
  that shape). Helpers such as `run_sss`, `run_common` and `assert_file_line`.
  `$BATS_TEST_TMPDIR` stands for `/` of the NAS: shared folders are created as
  `volume<N>/<share>` below it. It loads `bats-support` and `bats-assert`, which the
  `bats/bats` image ships.
- `fixtures/package.env` – brand values of a made-up product (`ID=demo`, `ENV_PREFIX=DEMO`), so the
  tests prove that every name comes from `package.env`. Its assignments are exactly what
  `spkmeta`'s `Package.PackageEnv` renders for that brand (Go test
  `TestScriptFixtureIsSpkmetaOutput` in `packaging/synology/cmd/spkstage`), so `common.sh` is
  tested against the format `spkstage` stages.
- `fake-server.bash` – the fake server.
- `fake-ffmpeg.sh` – the fake `bin/ffmpeg`: answers `-version`, `-demuxers`, `-decoders` and
  `-protocols` in FFmpeg's layout; `FAKE_FFMPEG_MISSING` leaves names out, `FAKE_FFMPEG_SILENT`
  makes it list nothing, `FAKE_FFMPEG_HANG` makes it hang (with `FFMPEG_CAPABILITIES_TIMEOUT=1`
  the tests show the timeout stops it).

### Shells

DSM's `/bin/sh` is not documented (`../DSM-NOTES.md` section 5). Every case of
`start-stop-status.bats` and `lifecycle.bats` therefore runs under four shells of the image:
BusyBox `sh` (ash, a minimal POSIX shell), `bash --posix` (what `/bin/sh` is when it links to
bash), `bash` and `dash` (a strict POSIX shell). A shell of the list that is missing in the image
fails one test instead of skipping its cases. For a one-off run with other shells, set
`SCRIPT_TEST_SHELLS` to a comma-separated list, for example Debian's dash:

```bash
docker run --rm -v "${PWD}:/code:ro" -w /code -e SCRIPT_TEST_SHELLS=dash debian:stable-slim \
  bash -c 'apt-get update -qq && apt-get install -y -qq bats bats-assert bats-support >/dev/null && bats packaging/synology/test/scripts/start-stop-status.bats packaging/synology/test/scripts/lifecycle.bats'
```

(`package-files.bats` needs jq, which that one-off image lacks.)

### The fake server

`start-stop-status` decides "running" by comparing `/proc/<pid>/exe` with the package binary,
so the fake must be a real executable at `bin/<id>`, not a shell script (a script's `exe` is its
interpreter). Each test therefore copies the `bash` executable to `bin/<id>` and exports
`BASH_ENV=fake-server.bash`: bash runs that file before anything else, and the file implements
the server and always exits. Every other bash that inherits `BASH_ENV` returns at its first line.

| Invocation | Behaviour |
|---|---|
| `<id>` | serves until `SIGTERM`; output on stdout and stderr |
| `<id> healthcheck` | exit 0 when the control file `health` says `ok`, else 1 |

Control files in `$FAKE_STATE_DIR` (written by the tests): `mode` = `healthy` (default), `slow`
(healthy after `FAKE_SLOW_SECONDS`, default 2), `unhealthy`, `exit-early` (30 log lines, exit 3)
or `ignore-term` (records `SIGTERM` and keeps running). Records written by the fake:
`server.starts` (one PID per start), `server.args`, `server.env`, `healthcheck.args`,
`healthcheck.env`, `server.signal`.

Two more tools of `helpers.bash` reach states the fake cannot: a log that is a FIFO without a
reader holds the forked shell before `exec` (the window between fork and exec), and
`fake_failing_command` puts an `rm` or `mv` first in `PATH` that fails for matching arguments
(the tests run as root, so file permissions cannot make them fail). The action lock cases of
`start-stop-status.bats` add their own: `start_fake_action` holds the lock as a live process
whose command line names the package's `start-stop-status` (optionally recorded for another
boot ID, or naming another package's script), `start_lock_holder` runs any holder script (for
example one that empties and rewrites the lock record, a new holder whose PID is not written
yet), `fake_slow_command` puts an `rm` first in `PATH` that sleeps before matching arguments (a
start that clears a large transcode directory), `fake_lock_race` a `mkdir` that fails once for
the lock without creating it (the holder released the lock between `mkdir` and the `-d` test),
and one case a `mkdir` that leaves a dangling link where the lock record goes, so writing it
fails also for root.

### Test-only variables of the package scripts

Documented in the script headers; DSM never sets them.

| Variable | Script | Replaces | Used by |
|---|---|---|---|
| `PKG_TEST_START_TIMEOUT_SECONDS` | `start-stop-status` | the 30 s start budget | start budget cases |
| `PKG_TEST_STOP_TIMEOUT_SECONDS` | `start-stop-status` | the 20 s stop budget before `SIGKILL` | "stop needing SIGKILL" case |
| `PKG_TEST_UPTIME_FILE` | `start-stop-status` | `/proc/uptime`, the clock of the start budget | "start budget" clock cases |
| `PKG_TEST_LOCK_WAIT_SECONDS` | `start-stop-status` | the 10 s wait for another action's lock, after which a lock whose holder is not the package's `start-stop-status` is broken | the lock cases |
| `PKG_TEST_LOCK_LIMIT_SECONDS` | `start-stop-status` | the 5 minute hard limit after which even a live action's lock is broken | "hard limit" case |
| `PKG_TEST_VOLUMES_ROOT` | `common.sh` (`PKG_VOLUMES_ROOT`): `preinst`, `postuninst` | `/`, where the `volume*` directories are: the shared folders `preinst` looks for and the `volume<N>/@appdata\|@apphome/<id>` shape `postuninst` deletes below | every case (set by `setup_package_tree`) |

### Timing assertions

Tests measure elapsed time with `/proc/uptime` (`monotonic_centiseconds`,
`assert_elapsed_at_least`, `assert_elapsed_below` in `helpers.bash`), never with bash `SECONDS`
or `date +%s`: under Docker Desktop (WSL2) the container's wall clock can lag behind, so two
`sleep 1` advanced `SECONDS` by only 0 or 1 s in a few runs. The fake server's `slow` mode uses
the same clock. `helpers.bash` sets `BATS_TEST_TIMEOUT` (60 s unless set in the environment), so
a hanging case fails instead of blocking the run.

## Adding tests

New `.bats` files load the helpers with `load helpers`, call `setup_package_tree` in `setup` and
`stop_leftover_processes` in `teardown`, write each case as a function `case_<name>` and register
the cases with `register_cases <name>...` so they run under every shell. Scripts are run with
`run_script <path> <args>` (`run_sss` for `start-stop-status`), `common.sh` functions with
`run_common <function> <args>`; stdout lands in `$output`, stderr in `$stderr`. Cases of scripts
that succeed call `refute_error_words`: the toolkit's log check and readers of DSM's package log
take `Error`, `error:` and `fatal error` for failures.

Files must use LF line endings (`.gitattributes` enforces it on checkout); a test fails on CR
characters in the scripts, the templates and this directory.

## Build script tests (`build/`)

`build/build-scripts.bats` tests the safety paths of `../build.sh` and `../build.ps1` (SPK-07,
SPK-16): CI hosts default to release builds, `--test` / `-TestBuild` overrides that, release
builds refuse interim or malformed `sources.lock` entries, the default version, a staged interim
or non-free FFmpeg, staged `ffmpeg`/`ffprobe` binaries or a `SOURCE.txt` version that differ
from the pins of `sources.lock` (a test build packs them with a warning and a line in
`NOT-FOR-RELEASE.txt`), a dirty git tree (also untracked files that
`status.showUntrackedFiles=no` hides), a failing git (a fake `git` reporting "dubious
ownership") and an empty revision (without git a release needs `--revision`; `run_build` adds
a fixture commit then, and the tree of each test is a committed checkout where git exists), an
spkstage warning, a toolkit run that writes no SPK, a staged DSM version with another package
revision, an invalid staged package name and option values that are empty or start with `-`; a
`SOURCE.txt` needs exactly one `Status:`, `Licence:`, `Version:` and `Source SHA-256:` line;
the staged FFmpeg source archive must be the one every `SOURCE.txt` names and lands in `dist/` with a `SHA256SUMS` line; the FFmpeg recipe
image is built before step 1 and passed to it; a failed toolkit run leaves no `SHA256SUMS`; and
the SPK names carry `-r<N>` for package revision N ≥ 1. `build/fetch.bats` tests
`../ffmpeg/fetch.sh` (install with modes and manifest, checksum and notice mismatches, missing
output files, usage errors, a failed recipe build, a licence that differs from the lock) under
the image's POSIX `sh`. `build/package-gates.bats` runs `../ffmpeg/build/scripts/package.sh`
against a fake `readelf`/`objdump`: a failing tool stops every gate, dynamic, foreign or too
new executables are refused, and `SOURCE.txt` is written only from checked values (one
well-formed `.tar.sha256` line `<sha256>  <name>` naming an archive that exists next to it, a
one-line configuration, 64-hex checksums). `build/stage.bats` runs `../stage.sh` (the staging
stage of the Dockerfile) against `build/fake-spkstage.bash`: build-info, the source archive,
the release refusals of step 1 and no temporary file left behind, also when it dies. A licence is always judged
on the whole `Licence:` value. `build/fake-docker.bash` replaces Docker (its header lists the
switches; the pins of the test locks are the SHA-256 of its fake binaries); its step 1 runs the
real `stage.sh` with the fake spkstage, so a release is refused there exactly as in the real
build (`FAKE_STAGE_TEST_MODE` lets step 1 pass to reach the checks of `build.sh` behind it).
Every case runs in well under a second. Each
build-script case runs against `build.sh` and, when `pwsh` is installed, against `build.ps1`
with the same arguments (parity). Each test copies both scripts into its own temp tree, so the
repository's `dist/` is never touched.

```powershell
# build.sh, stage.sh, fetch.sh and package.sh cases (the script test image has neither pwsh nor git; those cases are skipped)
docker run --rm -v "${PWD}:/code" -w /code spk-script-test:dev packaging/synology/test/build
# all cases, with the build.ps1 parity cases and the dirty-tree case
docker build -t spk-build-test:dev packaging/synology/test/build
docker run --rm -v "${PWD}:/code" -w /code spk-build-test:dev packaging/synology/test/build
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 packaging/synology/build.sh packaging/synology/stage.sh packaging/synology/test/build/build-scripts.bats packaging/synology/test/build/stage.bats packaging/synology/test/build/package-gates.bats packaging/synology/test/build/fetch.bats packaging/synology/test/build/fake-docker.bash packaging/synology/test/build/fake-spkstage.bash
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/ffmpeg/fetch.sh
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -x -s sh packaging/synology/ffmpeg/build/scripts/package.sh
```
