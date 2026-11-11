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

## Notes (M2-31)

Key added for the LAN-only guard (OD-10 A accepted):

| YAML key | Environment | Default | Validation |
|---|---|---|---|
| `server.lan_only` | `<PREFIX>_SERVER_LAN_ONLY` | `true` | `true` or `false`, case-insensitive; `yes`, `no`, `on`, `off`, `1`, `0` and an empty value are rejected so a security switch never flips on a guess |

- **What it does**: until accounts exist (milestone 7) the HTTP server answers every request
  whose socket peer is not in the local network with `403`, problem code `forbidden` and the
  fixed detail "Requests from outside the local network are refused (server.lan_only)."
  Local are loopback, RFC 1918, RFC 6598 (`100.64.0.0/10`, e.g. Tailscale), link-local and
  IPv6 unique local addresses; IPv4-mapped IPv6 peers count as their IPv4 address. `GET` and
  `HEAD` of `/api/v1/health` stay reachable for container health checks and monitoring. The
  startup line `http server listening` carries `lan_only=true|false`. `OPTIONS *` goes
  through the guard as well (`DisableGeneralOptionsHandler`).
- **On-link IPv6 prefixes**: dual-stack home networks (e.g. a FritzBox with an ISP prefix)
  give LAN clients only global and link-local IPv6 addresses, and browsers connect with the
  global one. The guard therefore also admits IPv6 peers inside a prefix of `/48` or longer
  on a host interface that is up and not loopback, provided the prefix lies in the global
  unicast space `2000::/3` and outside the tunnel ranges Teredo (`2001::/32`; on Windows the
  on-link /64 of the Teredo pseudo-interface holds every Teredo client of the same relay)
  and 6to4 (`2002::/16`). IPv4-compatible (`::/96`, Linux `sit0`), NAT64 (`64:ff9b::/96`),
  discard (`100::/64`), unique local, link-local and multicast prefixes are never taken from
  the interfaces; the private ones are covered by the fixed ranges. IPv4 interface subnets
  are never trusted (on a VPS they would admit the neighbours).
- **Refresh**: the prefixes come from an injected provider (`server.Options.InterfacePrefixes`,
  nil = the host's interfaces) and are reread at most every 30 s because ISP prefixes
  rotate. One request performs the refresh while all others keep deciding with the current
  snapshot; only the very first read blocks concurrent requests. When reading fails, the
  last known prefixes stay trusted for at most 4 consecutive failed refreshes (about two
  minutes) and are then dropped, so the guard fails closed. A failure streak logs one warning
  when it starts and one when the stale prefixes are dropped.
- **Only the socket peer counts** unless it is a trusted reverse proxy (next table):
  `X-Forwarded-For` and `X-Forwarded-Proto` from any other peer, and `X-Real-IP`,
  `Forwarded` and `X-Forwarded-Host` from every peer, are ignored.

Key added for trusted reverse proxies (ADR 0032 §3, task M7-40):

| YAML key | Environment | Default | Validation |
|---|---|---|---|
| `server.trusted_proxies` (list of CIDR prefixes or single addresses, IPv4 and IPv6) | `<PREFIX>_SERVER_TRUSTED_PROXIES` (comma-separated, spaces around entries ignored; replaces the YAML list as a whole; empty value = no proxy). Not an OS path list like `browse.roots`: its `:` separator outside Windows would cut IPv6 addresses apart | empty = trust nobody | every entry parses as an address or prefix (no port, no zone, no host name); a prefix of length 0 (`0.0.0.0/0`, `::/0`) and a duplicate are refused; entries are stored masked and IPv4-mapped IPv6 entries as IPv4, duplicates are compared after that. Errors wrap `ErrInvalid` and name `server.trusted_proxies[<index>]` and the entry |

- **What it does**: when the socket peer is inside the list, the client address is taken from
  `X-Forwarded-For` (right-most entry outside the list, else the left-most) and the scheme
  from the last `X-Forwarded-Proto` value. A listed proxy that sends no usable
  `X-Forwarded-For` (missing, `unknown`, more than 32 entries, an entry that does not parse
  before the client is found) counts as remote and keeps its own address. The resolved
  address and scheme decide `server.lan_only`, the local-network rule of setup and
  administration, the sign-in throttle address, the device's address class, the `Secure`
  cookie attribute and the same-origin scheme. Refusal logs carry `remote_address` (the
  resolved client) and `proxy_address`.
- **Trade-offs and known limits** (details for Docker users in `packaging/docker/README.md`):
  - Host-side clients of a Docker bridge network come from the private bridge gateway, so
    NAS setups keep working. The default bridge has no IPv6: Docker's proxy accepts IPv6 on
    `[::]:<port>` and forwards it over IPv4 from the gateway, so every IPv6 client, public
    ones included, passes. Publish IPv4 only (`-p 0.0.0.0:8460:8460`), use an IPv6-enabled
    network or `network_mode: host`. On-link IPv6 prefixes of the host are visible only with
    host networking.
  - A reverse proxy on the LAN, Docker Desktop, rootless Docker and tunnels on the same host
    (cloudflared, Tailscale Serve or Funnel, ngrok) show every client with a private or
    loopback address; behind them the guard lets everyone through.
  - Some VPS and hosting providers put many customers into one on-link /64. There the
    interface path admits those neighbours, the problem that makes the guard refuse IPv4
    subnets. This is a known limit: such servers must restrict access with a firewall or a
    proxy (keeping `lan_only` on still refuses everyone else).
  - Stateful DHCPv6 addresses appear as `/128` on Windows and with NetworkManager or
    systemd-networkd, so the interface yields no prefix that holds other hosts. On
    DHCPv6-only networks LAN clients that connect with a global IPv6 address get `403` (the
    safe direction); they can use IPv4 or a unique local address.
  - `100.64.0.0/10` is kept per OD-10 for Tailscale. It also admits carrier-grade-NAT
    neighbours at the ISP when the router's WAN side lies in shared space and the port is
    forwarded, devices shared into the tailnet from other tailnets, and cloud or Kubernetes
    networks that use the range.
- `false` turns the guard off; do that only when another layer restricts access.

## Notes (M3-11)

Keys added for the automatic scans (ADR 0027 §9; defaults are the OD-M3-3 recommendation):

| YAML key | Environment | Default | Validation |
|---|---|---|---|
| `scan.watch` | `<PREFIX>_SCAN_WATCH` | `true` | `true` or `false`, case-insensitive, like `server.lan_only`; `yes`, `no`, `on`, `off`, `1`, `0` and an empty value are rejected |
| `scan.interval` | `<PREFIX>_SCAN_INTERVAL` | `12h` | Go duration (`90m`, `1h30m`, `168h`); `0` (or `0s`) turns the periodic scan off, anything else must lie in 15m–168h. A bare number such as `12` is rejected because its unit would be a guess |

- **Watcher** (`scan.watch`): a change below a library root starts a normal full scan of that
  library once the library has been quiet for 30 s, at the latest 5 min after the first
  change; changes during a running scan lead to exactly one more scan after it. Hidden
  entries and NAS system folders are ignored like in the scan. Linux and other systems use
  one fsnotify watch per directory, Windows one recursive notification on the root, so
  folders below a library can still be renamed while the server runs.
- **When watching fails** (missing or unmounted root, a root that was renamed, the watch
  limit, lost events) the server logs one warning per library, retries every minute and
  relies on the periodic scan; once a retried watch is ready, one scan picks up what changed
  meanwhile. A root that goes away is noticed by its events or, at the latest, by a check of
  the root's identity every 30 s.
- **No warning for shares without events**: CIFS/SMB and NFS mounts on Linux, and bind
  mounts of Docker Desktop (Windows, macOS) from the host, accept watches but deliver no or
  only some events for changes made by other machines or by the host. The server cannot tell
  this apart from a quiet library, so it logs nothing; such libraries are kept current by the
  periodic scan alone. Do not turn `scan.interval` off for them.
- **inotify budget**: every watched directory costs one inotify watch, counted per user ID
  against `fs.inotify.max_user_watches` (and one instance per library against
  `fs.inotify.max_user_instances`). On a NAS, and for containers that run as root, that budget
  is shared with every other process of the same user ID, for example the NAS's own indexing
  and sync services. A library with tens of thousands of folders can exhaust it; the server
  then logs the watch-limit warning and keeps the folders it already watches. Raise the
  limits on the host (`sysctl fs.inotify.max_user_watches=...`) or rely on the periodic scan.
- **Periodic scan** (`scan.interval`): checked every minute; a library is scanned again when
  its last scan finished at least the interval ago. A library whose scan never finished
  since the server started counts from when it was first seen, so the startup scan (ADR
  0022) is not doubled. Network shares that send no events are covered only by this scan.
- Both keys can be off at the same time; then only the startup scan and manual scans run.

## Notes (ADR 0028)

`libraries` and `<PREFIX>_LIBRARIES` (M2-04) keep their validation but are a one-time import
seed into the database, not the live library list. New keys `browse.roots`
(`<PREFIX>_BROWSE_ROOTS`) and `install.kind` (`<PREFIX>_INSTALL_KIND`) follow in M3-19c.

## Notes (M3-19c)

| YAML key | Environment | Default | Validation |
|---|---|---|---|
| `browse.roots` (list of paths) | `<PREFIX>_BROWSE_ROOTS` (OS path list; replaces the YAML list as a whole; empty value = no roots) | Windows: every present drive root `A:\`..`Z:\`; others: those of `/media`, `/mnt`, `/srv` and the home directory that exist (probed at startup) | path rules above (relative → config directory, `~` and drive-relative refused); absolute and unique after cleaning (case-insensitive on Windows); a missing directory is not an error (reported unavailable later) |
| `install.kind` | `<PREFIX>_INSTALL_KIND` | `standard` | `standard`, `docker` or `synology`, case-insensitive; anything else is a startup error naming the key or variable |

`libraries` / `<PREFIX>_LIBRARIES` are an import seed: imported once at startup, then managed in
Settings > Libraries. Removing an entry from the configuration deletes nothing from the
database. Browse roots may contain the data or transcode directory; the browser hides it and
the domain refuses it, so the configuration applies no overlap rule to them.

Decision on the default browse roots: the home directory is added only for `install.kind:
standard` (desktop installs). On `docker` and `synology` it is skipped because a NAS or
container home may hold secrets or the data directory. It is never added when relative, when it
is the filesystem root (`HOME=/` in containers) or when it duplicates another default; the
value is cleaned first. `browse.roots` overrides all defaults.

## Notes (M6-05)

Keys added for online metadata (ADR 0031). Booleans are strict like `server.lan_only`.

| YAML key | Environment | Default | Validation |
|---|---|---|---|
| `metadata.online.enabled` | `<PREFIX>_METADATA_ONLINE_ENABLED` | `true` | `true` or `false`; `false` means no outbound connection at all |
| `metadata.tmdb.enabled` | `<PREFIX>_METADATA_TMDB_ENABLED` | `true` | `true` or `false` |
| `metadata.tmdb.api_key` | `<PREFIX>_METADATA_TMDB_API_KEY` | empty | Own TMDB key, no whitespace; empty clears it. Wins over the key bundled at build time |
| `metadata.musicbrainz.enabled` | `<PREFIX>_METADATA_MUSICBRAINZ_ENABLED` | `true` | `true` or `false`; also covers Cover Art Archive |
| `metadata.language` | `<PREFIX>_METADATA_LANGUAGE` | `en` | Language tag such as `en`, `de`, `pt-BR` (stored in canonical case) |

`Config.TMDBKey()` returns the key in use and its origin (`none`, `bundled`, `own`; own wins).
The key type `config.Secret` prints a redaction marker in `%v`, `%#v`, JSON and `slog`, and
error messages never contain the value. Keep it out of any config dump.
The per-library `online_metadata` flag is stored with the library (ADR 0031 §2), not here.

M6-14: the `metadata.*` keys are the starting state. Once the settings were changed in
Settings › Metadata they are stored in the database (`metadata_settings`, migration 00012)
and that stored state is used on every later start, so a UI change survives a restart, with
one exception: a switch set to `false` in the file or the environment (`metadata.online.enabled`,
`metadata.tmdb.enabled`, `metadata.musicbrainz.enabled`) is forced off while it is configured
so, whatever was stored, so an operator can always turn online lookups off. A forced switch is
never stored as the user's choice: (a) the UI shows it off, (b) a change in the UI is stored but
has no effect while the configuration forces it off, (c) removing the `false` from the
configuration brings back the user's stored choice. `metadata.language` stays the default an
empty language stands for.
