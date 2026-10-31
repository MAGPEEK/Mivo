# SPK-05 Server service control script, runtime environment and script test harness
Owner: mivo-synology
Depends on: SPK-01 (done)
Files/areas: `packaging/synology/package/scripts/start-stop-status`,
`packaging/synology/package/scripts/common.sh`, `packaging/synology/package/scripts/package.env`
(template), `packaging/synology/test/**` (new: harness, fake server, tests, `README.md`)

## Goal
Package Center can start, stop and query the media server (the `mivo` binary running as a DSM
service on port 8460) reliably: the server gets the DSM paths of ADR 0026 §6, a failed start is
reported instead of shown as "running", and a stop gives the server its graceful shutdown time.

## Acceptance criteria
- [ ] `common.sh` is located through the calling script's own directory (`${0%/*}`, never a fixed
      path), loads `package.env` from the same directory and derives every path of ADR 0026 §6
      from `SYNOPKG_PKGDEST`, `SYNOPKG_PKGVAR`, `SYNOPKG_PKGTMP` and `SYNOPKG_PKGHOME`. It exports
      `<ENV_PREFIX>_CONFIG_DIR`, `_PATHS_DATA`, `_PATHS_CACHE`, `_PATHS_TRANSCODE`
      (`$SYNOPKG_PKGTMP/transcode`, OD-SPK-7 A confirmed), `_TOOLS_FFMPEG`, `_TOOLS_FFPROBE` and
      `HOME=$SYNOPKG_PKGHOME`, built from `ENV_PREFIX`, never a literal prefix.
- [ ] `start-stop-status start`: creates data, cache and transcode directories (0750) if missing;
      removes stale entries below the transcode directory (it survives reboots and crashes);
      rotates `<id>.log` in `$SYNOPKG_PKGVAR` to `<id>.log.1` when larger than 10 MiB (named
      constant); starts `$SYNOPKG_PKGDEST/bin/<id>` in the background with its stdout and stderr
      appended to that log; writes the PID file; exits 0 once `<id> healthcheck` succeeds (polled,
      named interval). If the process exits first it exits 1 within the start budget and writes
      the last 20 log lines to `$SYNOPKG_TEMP_LOGFILE` when set. Alive but not healthy after 30 s
      → 0. Already running → 0 without a second process.
- [ ] `stop`: `SIGTERM`, waits up to 20 s (named constant), then `SIGKILL`; removes the PID file;
      exits 0, also when nothing runs.
- [ ] `status`: 0 when the PID file names a live process whose `/proc/<pid>/exe` is the package
      binary; 3 when not running, removing a stale PID file (dead process or PID reused by another
      program); 150 when the binary is missing or not executable.
- [ ] `prestart` and `prestop` exit 0 (DSM calls them by default, `prestart` also at boot with an
      empty `SYNOPKG_PKG_STATUS`); `log` exits 0; any other argument → 1 with a usage line on
      stderr.
- [ ] The script's own stdout stays short (a few lines per action; DSM appends it to
      `/var/log/packages/<id>.log`); the server's output goes only to `$SYNOPKG_PKGVAR/<id>.log`.
- [ ] Brand-free: `rg -i mivo packaging/synology/package` finds nothing; the test fixture
      `package.env` uses a different brand (e.g. `ID=demo`, `ENV_PREFIX=DEMO`) and the tests assert
      `DEMO_CONFIG_DIR` etc. reach the fake server.
- [ ] Test harness: a fake server (sh script) that records arguments and environment and can be told
      to become healthy, stay unhealthy, exit early or ignore `SIGTERM`; `healthcheck` answers from a
      control file. Every case runs under busybox `sh` and `bash`: start healthy, early exit (exit 1
      + log tail), slow start, double start, graceful stop, stop needing `SIGKILL` (shortened
      timeout through a test-only variable documented in the script), stale PID, missing binary
      (150), `prestart`/`prestop`/`log` (0), unknown argument (1), boot start with empty
      `SYNOPKG_PKG_STATUS`, stale transcode entries removed, log rotation, scripts called through a
      path other than the current directory (finds `common.sh`).
- [ ] One documented command runs all script tests in Docker, e.g.
      `docker run --rm -v "${PWD}:/code" -w /code bats/bats:<pinned tag> packaging/synology/test/scripts`,
      and passes on Docker Desktop (Windows).
- [ ] `docker run --rm -v "${PWD}:/mnt" -w /mnt koalaman/shellcheck:v0.11.0 -s sh packaging/synology/package/scripts/start-stop-status packaging/synology/package/scripts/common.sh`
      → no findings.
- [ ] Gates: shellcheck + script tests green; Go gates unaffected.

## Notes
- POSIX `sh` only, `set -eu`, every expansion quoted; read `/proc` instead of `pidof`/`pgrep`. The
  scripts run as the package user, never `sudo`. `/bin/sh` on DSM is unconfirmed (SPK-12 checks).
- `common.sh` and `package.env` live in `scripts/` (unconfirmed that DSM keeps extra files there;
  fallback: SPK-14 inlines them, so keep `common.sh` free of anything that breaks when pasted
  into a script).
- Do not set `server.port`, `server.lan_only` or libraries through the environment: they stay in
  `config.yaml` (ADR 0026 §6). Wizard keys never reach `start-stop-status`.
- LF line endings (`.gitattributes` forces LF; the harness fails on CR; the toolkit's `run.sh`
  refuses CRLF too).
