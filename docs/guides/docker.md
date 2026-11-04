# Running the server with Docker

This guide installs the media server as a container on a NAS or home server with
`docker run`. For Docker Compose, Synology Container Manager and QNAP Container Station, read
[docker-compose.md](docker-compose.md) after the sections "Before you start" and "Local
network only" of this page. The reference for every setting of the image is
[packaging/docker/README.md](../../packaging/docker/README.md).

Image: `docker.io/magpeek/mivo-media-server`, release `0.1.0-alpha.2`, for `linux/amd64`
(Intel and AMD) and `linux/arm64` (64-bit ARM). The tag `alpha` points to the newest alpha;
there is no `latest` tag, so always name a tag. The image runs without root as user
`1000:1000`, contains the server with the embedded web app and FFmpeg, and takes about
770 MB on disk (about 210 MB download).

## Local network only

**There are no user accounts and no login until milestone 7.** Everyone who can reach port
8460 can browse and play your videos. By default the server refuses clients outside your
local network (`MIVO_SERVER_LAN_ONLY=true`, ADR 0018), but that guard only sees the address of
the connection. Therefore:

- Do **not** forward the port on your router, and do not add the server to a reverse proxy
  (Synology's Application Portal and reverse proxy, QNAP's myQNAPcloud links, nginx or
  Traefik), a Cloudflare tunnel or Tailscale Funnel. Behind all of these every client looks
  local to the server, and the guard lets everyone in.
- Synology QuickConnect is not expected to reach the port at all (it serves DSM and
  Synology's own packages, see
  [packaging/synology/README.md](../../packaging/synology/README.md) and ADR 0026). Do not try
  to publish the server there.
- Keep `MIVO_SERVER_LAN_ONLY` at its default. Set it to `false` only when another layer
  controls who can reach the port.
- Publish the port for IPv4 only (`-p 0.0.0.0:8460:8460`): on Docker's default bridge
  network, IPv6 connections arrive from the bridge gateway and would pass the guard. Browsers
  fall back to IPv4 by themselves. The details, including Tailscale and `network_mode: host`,
  are in [packaging/docker/README.md](../../packaging/docker/README.md), "Local network only".

## Before you start

You need:

- Docker Engine with Docker Compose v2, Synology Container Manager or QNAP Container
  Station. Engine 25 or newer turns the container healthy within seconds; older engines,
  such as the one Container Manager on DSM 7.2 may ship, ignore the fast start probe and
  report healthy after about 30 s. `docker version` shows the engine version.
- Docker Desktop works for a trial on a PC, but there the LAN-only guard does not protect
  anything: Docker Desktop presents every client with a local address. Do not expose such a
  PC to other networks while the server runs.
- An x86-64 or 64-bit ARM machine. 32-bit ARM (`linux/arm/v7`) is not published.
- Three folders on the host. Create them before the first start; Docker would otherwise
  create them as root, which the container cannot write:

  | Host folder (your choice) | Container path | Purpose |
  |---|---|---|
  | `/srv/mivo/config` | `/config` | `config.yaml` (optional), `data/` (SQLite database), `cache/`. Keep it on local storage, not on an SMB or NFS share: the database lock may not work there. |
  | `/srv/videos` | `/media` | your videos. Mounted read-only; the server never writes into it. |
  | `/srv/mivo/transcode` | `/transcode` | temporary HLS output of remux and transcode sessions. Free space: the largest film times the number of concurrent sessions. Not a small `tmpfs`. |

- Port **8460** free on the host (web app and API).
- The user id the container runs as must be able to write the config and transcode
  folders and read the videos. See "User and permissions" below; on a NAS this step is
  mandatory.

## 1 Pull the image

```
docker pull docker.io/magpeek/mivo-media-server:0.1.0-alpha.2
docker run --rm docker.io/magpeek/mivo-media-server:0.1.0-alpha.2 --version
```

The second line prints `Mivo 0.1.0-alpha.2 (go1.27.3)`.

## 2 User and permissions

The container runs as user `1000:1000` unless told otherwise. Bind-mounted folders keep the
owner they have on the host, so the server can only use them if that user may write
`/config` and `/transcode` and read `/media`. Find out who owns your folders:

```
id <your user>          # e.g. uid=1026(stefan) gid=100(users)
ls -ln /srv/mivo        # numeric owner and group of the folders
```

Then pick one of two ways:

1. **Run the container as the owner** (recommended on a NAS): add `--user <uid>:<gid>`
   to `docker run`, for example `--user 1026:100` on Synology or `--user 1000:100` on QNAP.
   The user must not be `0` (root).
2. **Give the folders to the image's default user**: `sudo chown -R 1000:1000
   /srv/mivo/config /srv/mivo/transcode`, and make the videos readable for everyone
   (`chmod -R a+rX /srv/videos`) or for group `1000`.

There are no `PUID`/`PGID` variables yet; they arrive with milestone 9. `--user` is the
only switch today.

## 3 First start

```
docker run -d --name mivo \
  --user 1026:100 \
  -p 0.0.0.0:8460:8460 \
  --stop-timeout 20 \
  --restart unless-stopped \
  -e TZ=Europe/Berlin \
  -v /srv/mivo/config:/config \
  -v /srv/videos:/media:ro \
  -v /srv/mivo/transcode:/transcode \
  docker.io/magpeek/mivo-media-server:0.1.0-alpha.2
```

Replace `--user` with your ids (or drop it if you chose way 2 above) and the three host
paths with yours. In PowerShell write the command on one line or continue lines with a
backtick instead of `\`.

What the options do:

- `--stop-timeout 20`: the server shuts down gracefully within 18 s (finishes open requests,
  stops FFmpeg, closes the database). Docker's default of 10 s would kill it in the middle.
- `--restart unless-stopped`: start again after a reboot or a crash, but not after you stop
  it yourself.
- `-e TZ=…`: time zone for the log. Every other setting is a `MIVO_*` variable; see
  "Settings" below.
- `/media:ro`: read-only; nothing in the container needs to write there.

Check that it runs:

```
docker ps --filter name=mivo       # STATUS "Up … (healthy)" after a few seconds (about 30 s on Engine < 25)
docker logs mivo                   # "http server listening" and "library scan finished"
```

The image has a built-in health check (`mivo healthcheck`, which calls
`/api/v1/health`), probed every 2 s during the first minute (Docker Engine 25 or newer;
older engines probe only every 30 s) and every 30 s afterwards.
`(health: starting)` for longer than a minute or `(unhealthy)` means something is wrong; see
"Troubleshooting".

## 4 Open the web app

Open `http://<address of the host>:8460` in a browser on the same network, for example
`http://192.168.1.10:8460`. The sidebar lists one library named `media` (the `/media`
mount), scanned at every start. The grid fills while the first scan runs; the scan button in
the web app scans again later. The API is under `/api/v1`, for example
`http://192.168.1.10:8460/api/v1/system/info`.

## 5 Settings

Settings are environment variables (`-e NAME=value`), or keys in `/config/config.yaml`
(ADR 0018: defaults < `config.yaml` < environment). The container must be recreated for a
change (`docker rm -f mivo`, then the `docker run` again; the data in `/config` stays).

| Variable | Default | Meaning |
|---|---|---|
| `MIVO_LIBRARIES` | `/media` | library roots inside the container, separated by `:` |
| `MIVO_TRANSCODE_MAX_SESSIONS` | `2` | concurrent re-encoding sessions, `1`–`8`; use `1` on a weak NAS |
| `MIVO_SERVER_PORT` | `8460` | port inside the container; change the `-p` mapping as well |
| `MIVO_SERVER_LAN_ONLY` | `true` | refuse clients outside the local network (keep it) |
| `MIVO_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `MIVO_LOG_FORMAT` | `text` | `text` or `json` |
| `TZ` | UTC | time zone of the log timestamps |

Do not append server flags such as `--config-dir` to `docker run`: the health check is a
second process that reads only the environment and `/config` and would then report the
container as unhealthy. `MIVO_CONFIG_DIR` and `MIVO_PATHS_*` are set by the image and
should stay as they are. Leave `MIVO_TOOLS_FFMPEG` and `MIVO_TOOLS_FFPROBE` unset: the
image's FFmpeg is on `PATH` and found by name.

## 6 Adding more libraries

Mount every folder below `/media` and list the container paths in `MIVO_LIBRARIES`. Each
becomes a `movies` library named after its last folder:

```
docker run -d --name mivo ... \
  -v /srv/videos/Movies:/media/movies:ro \
  -v /srv/videos/Series:/media/series:ro \
  -e MIVO_LIBRARIES=/media/movies:/media/series \
  docker.io/magpeek/mivo-media-server:0.1.0-alpha.2
```

Rules: roots must not be nested in each other (do not list `/media` together with folders
below it), and an empty `MIVO_LIBRARIES=` means no library at all. A folder that does not
exist yet (a share that mounts late) is accepted and shown as unavailable until it appears.

For display names of your own or the kind `shows` (listed like movies until milestone 3),
put the list into `/config/config.yaml`:

```yaml
libraries:
  - name: Movies
    path: /media/movies
  - name: Series
    path: /media/series
    kind: shows
```

The image's `MIVO_LIBRARIES=/media` replaces the file's list as a whole, and Docker cannot
unset an image variable, so start the server through `env -u` so that only the server
process loses the variable (the health check keeps it, which is harmless):

```
docker run -d --name mivo ... \
  --entrypoint /usr/bin/env \
  docker.io/magpeek/mivo-media-server:0.1.0-alpha.2 -u MIVO_LIBRARIES /usr/local/bin/mivo
```

This workaround is temporary; a lower-priority image default is planned before milestone 9
(`docs/tasks/BACKLOG.md`).

## 7 Updating

Updates are a new tag. Pull it, remove the old container and create it again with the same
command; `/config` keeps the settings and the database, which the new version migrates at
start. Downgrading to an older version is not supported (a newer database schema stays), so
back up `/config` first (next section).

```
docker pull docker.io/magpeek/mivo-media-server:<new tag>
docker rm -f mivo
docker run -d --name mivo ... docker.io/magpeek/mivo-media-server:<new tag>
```

Whether you pin `0.1.0-alpha.2` or follow `alpha`, a pull is only applied when the container
is recreated. Alpha releases may change behaviour between versions; read the release notes
on <https://github.com/MAGPEEK/Mivo/releases> first.

## 8 Logs

The server logs to the container's standard error, so `docker logs` shows everything:

```
docker logs mivo                 # whole log
docker logs -f --tail 100 mivo   # follow
```

Lines are `time=… level=INFO msg="…" key=value …`. At a healthy start you see, in this
order: `starting Mivo` with the version, `applied database migration` (first start),
`ffmpeg ready for direct play, remux and transcode`, `libraries synced from the
configuration`, `http server listening … lan_only=true` and, a moment later, `library scan
finished … added=N failed=0`. `MIVO_LOG_LEVEL=debug` adds detail; `MIVO_LOG_FORMAT=json`
writes one JSON object per line for log collectors.

## 9 Backup

Everything the server owns is in `/config`: `config.yaml` if you wrote one, `data/` with
the SQLite database and `cache/`. Stop the container before copying so the database is
consistent, or copy while it runs and accept that the last seconds may be missing:

```
docker stop mivo
tar -czf mivo-config-$(date +%F).tgz -C /srv/mivo config
docker start mivo
```

`/transcode` holds only temporary output and needs no backup; `/media` is your own data.
Restoring is copying the folder back and starting the container with the same version or a
newer one.

## 10 Troubleshooting

**The container exits right away, `docker logs mivo` ends with
`mivo: load configuration: read /config/config.yaml: open /config/config.yaml: permission denied`.**
The server cannot read or write `/config`: the host folder belongs to another user than the
one the container runs as (this is the usual case on a Synology or QNAP, where the folders
belong to your NAS user, not to `1000`). Fix it with `--user <uid>:<gid>` of the owner or by
`chown` of the folder (section 2), then `docker rm -f mivo` and run again. With
`--restart unless-stopped` Docker keeps restarting the failing container, so `docker ps` shows
it as "Restarting".

**The server starts but the log shows
`level=WARN msg="leftover playback session directories could not be deleted" error="… open /transcode: permission denied"`
and every film that is not Direct Play fails with a stream error.** `/transcode` is not
writable by the container's user. Same fix as above.

**The library stays empty and the log shows
`level=WARN msg="library scan finished" … state=failed … code=root_unavailable cause="library folder is missing or unreadable"`.**
The container's user cannot read the mounted videos, or the host path of the `/media`
mount is wrong or empty. Check `docker exec mivo ls -ln /media`: it must list your files
with permissions the user can read (`chmod -R a+rX` on the host, or `--user` of the owner).

**`docker run` fails with `Bind for 0.0.0.0:8460 failed: port is already allocated`** (or
"address already in use"). Another program, an older container or a second instance uses
the port. `docker ps -a` shows containers; `ss -ltnp | grep 8460` (Linux) shows other
listeners. Either stop it or publish another host port: `-p 0.0.0.0:9000:8460` (the
container port stays 8460), then open `http://<host>:9000`.

**`docker ps` shows `(unhealthy)` or `(health: starting)` for more than a minute.** The
health check cannot reach the server on its port. Look at `docker logs mivo` for a startup
error; most are one of the permission cases above. If you set `MIVO_SERVER_PORT`, the check
follows it; if you appended `--config-dir` or other flags to the run command, remove them.
`docker inspect --format '{{json .State.Health}}' mivo` prints the last probe output.

**The web app opens, but nothing plays.** Open the player's stream info overlay or look at
`docker logs mivo` while you press play:

- `transcode_unavailable`: FFmpeg could not start. The image bundles FFmpeg on `PATH`, so
  this usually means `MIVO_TOOLS_FFMPEG` or `MIVO_TOOLS_FFPROBE` was set; remove them. The start log then shows `ffmpeg not found: only direct play available`.
- `session_limit_reached` (HTTP 503): the sessions are busy; wait, or raise
  `MIVO_TRANSCODE_MAX_SESSIONS` if the machine has the CPU for it.
- A stream error with `permission denied` in the log: `/transcode` is not writable, see
  above.
- Only one browser cannot play while another can: the browser lacks the codec, and the
  server did not transcode because the file is listed as Direct Play; check the overlay's
  reason. Transcoding is software-only and may be too slow for 4K on a small NAS.
- Playback from a phone on mobile data or from outside the house: refused on purpose
  (`403`, "Requests from outside the local network are refused"). See "Local network only".

**Everything looks fine, but `http://<host>:8460` does not load from another device.** The
host's firewall blocks the port (Synology: Control Panel > Security > Firewall; Linux:
`ufw`, `firewalld`), or you used `-p 127.0.0.1:8460:8460`, which binds the port to the host
itself only.

**Stopping takes 10 s and the log ends abruptly.** Add `--stop-timeout 20`; Docker's default
is too short for the graceful shutdown.

For anything else, open an issue at <https://github.com/MAGPEEK/Mivo/issues> with the output
of `docker logs mivo` and your `docker run` command (without passwords; there are none yet).
