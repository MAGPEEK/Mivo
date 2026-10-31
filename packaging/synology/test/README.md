# Package script tests

Tests of the DSM package scripts in `../package/scripts/` (task SPK-05: `start-stop-status`,
`common.sh`, `package.env`; the lifecycle scripts of SPK-06 join here). They run with
[bats-core](https://github.com/bats-core/bats-core) in Docker against a fake server, so they
need neither DSM nor the real server binary.

## Run

From the repository root; Docker Desktop is enough, nothing else is installed.

```powershell
# PowerShell
docker run --rm -v "${PWD}:/code" -w /code bats/bats:1.14.0 packaging/synology/test/scripts
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/package/scripts/start-stop-status packaging/synology/package/scripts/common.sh packaging/synology/package/scripts/package.env
docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 packaging/synology/test/scripts/start-stop-status.bats packaging/synology/test/scripts/helpers.bash packaging/synology/test/fake-server.bash packaging/synology/test/fixtures/package.env
```

In Git Bash, prefix each command with `MSYS_NO_PATHCONV=1` and use `"$(pwd -W)"` instead of
`"${PWD}"`, so that `/code` and `/mnt` are not rewritten into Windows paths. On Linux and in CI
the PowerShell lines work unchanged in bash.

The bats run takes about a minute: every case runs three times, and a few cases wait for the
shortened start and stop budgets. `--filter 'stop.*busybox'` (a regular expression over the test
names) runs a subset.

## What runs where

- `scripts/*.bats` – the test files; bats runs every `.bats` file of the directory.
  `start-stop-status.bats` covers SPK-05.
- `scripts/helpers.bash` – shared setup: a DSM-like package tree per test in `$BATS_TEST_TMPDIR`
  (`target` is a link into `volume1/@appstore/<id>` as on DSM; `var`, `tmp`, `home`; `scripts/`
  with the scripts under test and the fixture `package.env`), the `SYNOPKG_*` variables, and
  helpers such as `run_sss` and `assert_file_line`. It loads `bats-support` and `bats-assert`,
  which the `bats/bats` image ships.
- `fixtures/package.env` – brand values of a made-up product (`ID=demo`, `ENV_PREFIX=DEMO`), so the
  tests prove that every name comes from `package.env`.
- `fake-server.bash` – the fake server.

### Shells

DSM's `/bin/sh` is not documented (`../DSM-NOTES.md` section 5). Every case of
`start-stop-status.bats` therefore runs under three shells of the image: BusyBox `sh` (ash, a
minimal POSIX shell), `bash --posix` (what `/bin/sh` is when it links to bash) and `bash`. For a
one-off run with other shells, set `SCRIPT_TEST_SHELLS` to a comma-separated list, for example
Debian's dash:

```bash
docker run --rm -v "${PWD}:/code:ro" -w /code -e SCRIPT_TEST_SHELLS=dash debian:stable-slim \
  bash -c 'apt-get update -qq && apt-get install -y -qq bats bats-assert bats-support >/dev/null && bats packaging/synology/test/scripts'
```

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

### Test-only variables of `start-stop-status`

Documented in the script header; DSM never sets them.

| Variable | Replaces | Used by |
|---|---|---|
| `PKG_TEST_START_TIMEOUT_SECONDS` | the 30 s start budget | "alive but not healthy" case |
| `PKG_TEST_STOP_TIMEOUT_SECONDS` | the 20 s stop budget before `SIGKILL` | "stop needing SIGKILL" case |
| `PKG_TEST_UPTIME_FILE` | `/proc/uptime`, the clock of the start budget | "start budget" clock cases |

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
`run_script <path> <args>` (`run_sss` for `start-stop-status`); stdout lands in `$output`,
stderr in `$stderr`.

Files must use LF line endings (`.gitattributes` enforces it on checkout); a test fails on CR
characters in the scripts and in this directory.
