# 0018 Configuration (YAML + env overrides) and OS-specific paths

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-29
- Refines: tech stack row "Logging/Config" in `docs/plan.md`

## Context

The same binary runs natively on Windows, natively on Linux and inside Docker on a NAS.
Docker users configure through environment variables. Native users expect a config file in
the usual OS location.

## Decision

- **Precedence** (later wins): built-in defaults < `config.yaml` < environment variables.
  Only the config directory itself also comes from a command-line flag.
- **Config directory**: `--config-dir` flag > `<PREFIX>_CONFIG_DIR` env > OS default. The
  file `config.yaml` in it is optional. If it is missing, defaults apply and no file is
  written.
- **Strict parsing**: unknown YAML keys and invalid values are startup errors that name the
  key (or environment variable) and the file. Relative paths in the file resolve against the
  config directory.
- **Environment names**: brand env prefix + `_` + key path in upper snake case, for example
  `server.port` → `MIVO_SERVER_PORT`. The prefix comes from `internal/platform/brand`.
- **Milestone 1 keys**: `server.host` (default empty = all interfaces), `server.port`
  (default **8460**, 0 = OS-chosen for tests), `log.level` (debug|info|warn|error, default
  info), `log.format` (text|json, default text), `paths.data`, `paths.cache`,
  `paths.transcode`.
- **OS defaults** (directory name from brand: display name on Windows, lowercase id on Linux):

  | | config | data | cache | transcode |
  |---|---|---|---|---|
  | Windows | `%LOCALAPPDATA%\Mivo` | `…\Mivo\data` | `…\Mivo\cache` | `…\Mivo\transcode` |
  | Linux | `$XDG_CONFIG_HOME/mivo` (`~/.config/mivo`) | `$XDG_DATA_HOME/mivo` (`~/.local/share/mivo`) | `$XDG_CACHE_HOME/mivo` (`~/.cache/mivo`) | `<cache>/transcode` |
  | Docker | `/config` | `/config/data` | `/config/cache` | `/transcode` (set through env in the image) |

  Service installs (Windows service under `%ProgramData%`, systemd under `/var/lib`) come in
  milestone 9 and are set through env or flags.
- **Libraries**: `go.yaml.in/yaml/v3` with known-fields checking. No viper or koanf, to keep
  dependencies small.
- The config is a plain struct passed by value. `app` and `domain` never import it. `cmd`
  hands the relevant values to constructors.

## Consequences

- The default port 8460 avoids Jellyfin (8096), Plex (32400) and the common *arr ports.
  Change it here if the owner prefers another one.
- New settings must document their YAML key, env name, default and validation in the task
  that adds them.

## Notes (M1-04)

Clarifications from the implementation (M1-04, M1-04b); they refine the decision above
without changing it.

- **Relative paths from the environment** resolve against the config directory too, not
  against the working directory, so `<PREFIX>_PATHS_DATA=data` means `<config dir>/data`. Only
  the config directory itself (`--config-dir`, `<PREFIX>_CONFIG_DIR`) resolves against the
  working directory. Every directory in the loaded config is absolute.
- **Defaults do not follow `--config-dir`**: data, cache and transcode keep their OS
  defaults when only the config directory is moved. Set `paths.*` (or the env variables) to
  move them as well.
- **Docker** sets data, cache and transcode explicitly through env in the image
  (`<PREFIX>_PATHS_DATA=/config/data`, `<PREFIX>_PATHS_CACHE=/config/cache`,
  `<PREFIX>_PATHS_TRANSCODE=/transcode`); the Docker column above is not computed by the
  server.
- **`~` is never expanded**: only shells do that. A path starting with `~` in the file, in an
  env variable or as config directory is a startup error, so an unquoted or quoted `~` never
  turns into a directory literally named `~`.
- **Windows paths without a drive**: a rooted path such as `\data` resolves against the
  volume of the config directory (`C:\data` for a config directory on `C:`), like Windows
  resolves it against the current drive. A drive-relative path such as `C:data` is rejected
  because its meaning depends on hidden per-drive state.
- **Unusable OS variables**: relative or empty `LOCALAPPDATA`, `XDG_*_HOME` and home
  directory values (`USERPROFILE`, `HOME`) are ignored, as the XDG specification requires.
  Each default directory is computed on its own, so a missing `XDG_CONFIG_HOME` and `HOME`
  only fails the directories that need them; the error names the variables.
- **Strict YAML**: an explicit null (`data:` without value, `~`, `null`) is an error rather
  than "use the default" – remove the key instead. Duplicate keys and a second YAML document
  (`---`) are errors. Unknown keys are reported as `<file>: line N: unknown key "server.prot"`.
- **Injected environment**: the variable lookup (including the home directory) is passed in
  by `cmd`, so tests and the startup in M1-10 never mix an injected environment with the
  variables of the host.
