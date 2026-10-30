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
