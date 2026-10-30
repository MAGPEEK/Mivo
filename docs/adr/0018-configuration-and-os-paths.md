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
- **How strictness is implemented**: `config.yaml` is decoded into a `yaml.Node` tree and
  walked against the explicit key table, not decoded into structs with `KnownFields`. The
  walk is what tells an absent key from an explicit null and reports unknown keys with their
  full key path and line. An empty or null *section* (`server:` without keys) is allowed and
  sets nothing; only a null *value* is an error.

## Notes (M2-04)

Keys added in milestone 2 (ADR 0022, ADR 0024; OD-1 A and OD-4 A accepted):

| YAML key | Environment | Default | Validation |
|---|---|---|---|
| `libraries` (list of `{name, path, kind}`) | `<PREFIX>_LIBRARIES` (OS path list) | empty | see below |
| `tools.ffmpeg` | `<PREFIX>_TOOLS_FFMPEG` | `ffmpeg` | not empty or blank; a bare name stays for the `PATH` lookup at startup, a value with a path separator or drive follows the path rules |
| `tools.ffprobe` | `<PREFIX>_TOOLS_FFPROBE` | `ffprobe` | as `tools.ffmpeg` |
| `transcode.max_sessions` | `<PREFIX>_TRANSCODE_MAX_SESSIONS` | `2` | integer 1–8; counts re-encoding sessions only. The maximum is the fixed cap of 8 FFmpeg sessions (ADR 0024), so no accepted value is ineffective |

- **Library entries**: `path` is required and follows the path rules above (relative →
  config directory, `~` and drive-relative rejected). `name` defaults to the last element of
  the resolved path (the whole path for a volume root) and must not be blank. `kind` is
  `movies` (default) or `shows`, case-insensitive. Unknown fields are reported like other
  unknown keys (`line N: unknown key "libraries[0].pth"`), errors name the entry as
  `libraries[i]`.
- **The environment replaces the whole list**: `<PREFIX>_LIBRARIES` is split with the OS
  list separator (`;` on Windows, `:` elsewhere); each element becomes a `movies` library
  named after its last path element. An empty value sets an empty list, so it can switch off
  a list from the file or an image preset. Lists are never merged entry by entry.
- **Overlap rules**: after resolution, roots must be unique, not nested in each other, and
  neither be, contain nor lie inside the config, data, cache or transcode directory. On
  Windows the comparison ignores case and treats `/` like `\`. Symlinks are not evaluated,
  because a root need not exist at startup (NAS shares mount late, ADR 0022). Since relative
  roots resolve against the config directory, a useful relative root starts with `..`.
- **Drive-relative config directory**: `--config-dir C:cfg` and `<PREFIX>_CONFIG_DIR=C:cfg`
  are rejected on Windows like drive-relative values in the file.
