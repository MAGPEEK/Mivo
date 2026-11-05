# Docker image

The container image of the media server (ADR 0014). It holds one static binary with the
embedded web app (ADR 0020) and Debian's FFmpeg (interim build, ADR 0015). This file is the
reference for every setting of the image; the step-by-step user guides are
[docs/guides/docker.md](../../docs/guides/docker.md) (`docker run`) and
[docs/guides/docker-compose.md](../../docs/guides/docker-compose.md) (Compose, Synology
Container Manager, QNAP Container Station) with the ready-made
[docker-compose.yml](docker-compose.yml) next to this file.

Published image: `docker.io/magpeek/mivo-media-server`, tags `0.1.0-alpha.5` and `alpha`
(no `latest`), platforms `linux/amd64` and `linux/arm64`. The examples below use the
published tag; replace it with `mivo:dev` for an image you built yourself.

## Build

From the repository root:

```
docker build -f packaging/docker/Dockerfile -t mivo:dev .
docker build -f packaging/docker/Dockerfile --build-arg VERSION=0.1.0-alpha.1 --build-arg REVISION=$(git rev-parse HEAD) -t mivo:0.1.0-alpha.1 .
```

The builder stages run on the build machine and cross-compile, so multi-arch builds need no
emulation for Node or Go (publishing is milestone 9):

```
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -f packaging/docker/Dockerfile .
```

`Dockerfile.dockerignore` limits the build context to `go.mod`, `go.sum`, `cmd/`, `internal/`
and `web/` (without `node_modules`, build output, coverage and Playwright output), plus the
FFmpeg capability check `packaging/synology/package/scripts/ffmpeg-capabilities.sh`.

The build fails when Debian's FFmpeg lacks a capability that folder discs, ISO images or photo
thumbnails need (tasks M3-16j, M3-16j-b): the `concat`, `mpeg` and `mpegts` demuxers, the
`file`, `concat` and `subfile` input protocols, or one of the decoders `mjpeg`, `png`, `webp`,
`gif`, `tiff` (Debian 13's FFmpeg 7.1.5 has all of them). The script is bind-mounted for that
step only, so no image layer keeps it. The same step reports,
without failing, whether `hevc`, HEIF (`mov` demuxer plus `hevc`) and `libdav1d` decode is
available; the report is in the build log and in the image:

```
docker run --rm --entrypoint cat mivo:dev /usr/local/share/doc/ffmpeg-capabilities.txt
```

## Run

```
docker run -d --name mivo -p 0.0.0.0:8460:8460 --stop-timeout 20 \
  -v /path/to/config:/config -v /path/to/media:/media:ro -v /path/to/transcode:/transcode \
  docker.io/magpeek/mivo-media-server:0.1.0-alpha.5
```

`--stop-timeout 20` (compose: `stop_grace_period: 20s`) gives the server time for its
graceful shutdown, which is bounded to 18 s in total: up to 10 s for open requests, up to 5 s
to stop the playback sessions (FFmpeg killed, session directories removed), up to 2 s for
running library scans, then the database is closed. Docker's default of 10 s would kill it
with `SIGKILL` in the middle of that.

The image needs no init process: the server handles `SIGTERM` and `Ctrl+C` itself. When you
run it interactively (`docker run -it`), add `--init` so that a second `Ctrl+C` ends a
shutdown that hangs; as PID 1 the server would otherwise ignore it.

| Path / setting | Purpose |
|---|---|
| `/config` | `config.yaml` (optional), `data/` (database) and `cache/` |
| `/media` | where your videos are mounted; the web app browses it when you add a library (`MIVO_BROWSE_ROOTS=/media`), read only is enough |
| `/transcode` | temporary output of remux and transcode sessions |
| port `8460` | web app and REST API |
| `MIVO_LIBRARIES` | optional: library folders to import once at the first start (see below) |
| `MIVO_TRANSCODE_MAX_SESSIONS` | concurrent re-encoding sessions, default `2` (1 on hosts with 4 or fewer CPUs), allowed `1`–`8` |
| `MIVO_*` | every setting of ADR 0018, e.g. `MIVO_LOG_LEVEL=debug` |
| `TZ` | time zone, e.g. `TZ=Europe/Berlin` (tzdata is installed) |

`HEALTHCHECK` runs `mivo healthcheck`, which GETs `/api/v1/health` on the configured port.

### Libraries

The image starts without a library. Mount your media below `/media` (for example
`-v /volume1/video/movies:/media/movies:ro -v /volume1/video/series:/media/series:ro`), open
the web app and add the folders in **Settings > Libraries** (ADR 0028). The folder browser
starts at `/media` (`MIVO_BROWSE_ROOTS=/media`, `MIVO_INSTALL_KIND=docker` are set by the
image). The server scans a library when you add or change it, at every start and
periodically. Managing libraries works from the local network only until accounts exist.

To seed libraries from the configuration instead, list the container paths in
`MIVO_LIBRARIES`, separated by `:`, or use `libraries` in `/config/config.yaml` (names and
kinds need the file):

```
docker run ... -e MIVO_LIBRARIES=/media/movies:/media/series docker.io/magpeek/mivo-media-server:<version>
```

Configured libraries are imported **once per folder**: a library you delete in the app is not
imported again at the next start, and the configuration never changes or deletes a library you
manage in the app. The image no longer sets `MIVO_LIBRARIES`.

**Upgrading from 0.1.0-alpha.2**: that version made `/media` one library from
`MIVO_LIBRARIES=/media`. After the upgrade the catalog is rebuilt anyway (ADR 0027) and the
new image has no library: add your library once in **Settings > Libraries**, or start the new
image with `-e MIVO_LIBRARIES=/media` to get the old library back.

### Transcoding

Direct play streams the original file. Remux and transcode sessions run Debian's `ffmpeg`
and write HLS segments to `/transcode/sessions/<session>`; a session's directory is removed
when the player stops it, after 2 minutes without requests, at shutdown, and leftovers at the
next start.

- **Limit**: at most `MIVO_TRANSCODE_MAX_SESSIONS` sessions re-encode at the same time
  (default `2`, `1` on hosts with 4 or fewer CPUs; the CPU count ignores `--cpus` quotas, so
  set it by hand in a CPU-limited container), and at most 8 sessions run FFmpeg at all,
  remuxing included. A request beyond a limit gets `503` with problem code
  `session_limit_reached` and `Retry-After`. Expert transcode settings (`MIVO_TRANSCODE_PRESET`,
  `_THREADS`, `_PRIORITY`, `_THROTTLE_AHEAD_SECONDS`, `_HWACCEL`, `_HWACCEL_DEVICE`,
  `_ALLOW_BURN_IN`) are described in `docs/development.md`.
- **Space**: there is no throttling until milestone 4. FFmpeg processes the whole file as
  fast as it can, not as fast as the player watches, so one session can fill `/transcode` up
  to roughly the size of the film (remux) or of its 1080p H.264 version (transcode). Give
  `/transcode` free space for the largest film times the number of concurrent sessions, and
  do not put it on a small `tmpfs`.
- **FFmpeg build**: the image uses Debian's `ffmpeg` package (FFmpeg 7.1 on Debian 13) as the
  interim build until jellyfin-ffmpeg arrives with hardware acceleration in milestone 4 or
  with distribution in milestone 9 (ADR 0015). It encodes with `libx264` and `aac` on the CPU;
  hardware encoders are not used yet. The server logs at start what the build can do
  (`ffmpeg ready for direct play, remux and transcode`). A build without these encoders
  still serves direct play and remux and answers transcode requests with
  `transcode_unavailable`.

### Verify an image (release gate)

`go test` checks playback with the FFmpeg of the dev machine; the playback smoke test checks
the image's FFmpeg (ADR 0025). Generate the corpus, mount it at `/media` and run the smoke test
from the repository root (needs a local `ffprobe`). A fresh image has no library, so the smoke
creates one over `/media` through `POST /api/v1/libraries`:

```
go run ./scripts/gen-testmedia
docker run -d --name mivo-smoke -p 127.0.0.1:8460:8460 -v "$PWD/testdata/media:/media:ro" mivo:dev
go run ./scripts/playback-smoke -base http://127.0.0.1:8460
go run ./scripts/playback-smoke -base http://127.0.0.1:8460 -profile firefox-desktop
docker exec mivo-smoke ls /transcode/sessions    # empty: every session was removed
docker rm -f -v mivo-smoke
```

It prints one line per corpus video and seek case and a summary, and exits with `1` on any
mismatch. To play the DVD folder of the disc corpus through a session as well (M3-16k),
mount both trees below `/media` and pass the library manifest:

```
docker run -d --name mivo-smoke -p 127.0.0.1:8460:8460 -v "$PWD/testdata/media:/media/corpus:ro" -v "$PWD/testdata/library/discs:/media/discs:ro" mivo:dev
go run ./scripts/playback-smoke -base http://127.0.0.1:8460 -disc-manifest testdata/library/manifest.json
```

On a Linux host, `gen-testmedia` writes the corpus with mode `0640`, which user
`1000` of the container cannot read unless it owns the files; add
`--user "$(id -u):$(id -g)"` to `docker run` (the anonymous `/config` and `/transcode`
volumes are then not writable, so also mount writable host directories there) or run it
against a world-readable copy (`cp -r testdata/media /tmp/corpus && chmod -R a+rX /tmp/corpus`).

### Local network only

The server has no accounts yet (milestone 7), so by default it refuses requests from outside
the local network with `403` (`MIVO_SERVER_LAN_ONLY=true`, ADR 0018); `/api/v1/health` stays
reachable. It admits loopback, private (RFC 1918, IPv6 ULA), link-local and `100.64.0.0/10`
addresses, plus IPv6 addresses inside a global prefix (`/48` or longer, without the Teredo
and 6to4 tunnel ranges) of the server's own network interfaces, because dual-stack home
networks give their clients global IPv6 addresses. If those prefixes cannot be read for
about two minutes, such clients are refused. Only the address of the connection counts;
`X-Forwarded-For` is ignored.

What Docker networking does to that address:

- **IPv4 on the default bridge network (Linux, NAS)**: the container sees the real address
  of clients from the LAN or the internet; connections from the host itself come from the
  bridge gateway, a private address. LAN setups keep working.
- **IPv6 on the default bridge network**: the default bridge has no IPv6, so Docker's proxy
  accepts IPv6 connections on `[::]:8460` and forwards them over IPv4 from the bridge
  gateway (`172.17.0.1`). Every IPv6 client, public ones included, then passes the guard.
  Publish the port for IPv4 only (`-p 0.0.0.0:8460:8460`, compose `"0.0.0.0:8460:8460"`;
  browsers fall back to IPv4), or use an IPv6-enabled network or `network_mode: host`.
- **Global IPv6 addresses of LAN clients** are recognised only with `network_mode: host`: in
  a bridge container the server sees the container's interfaces, not the host's prefix, so
  on an IPv6-enabled bridge network such clients get `403` unless they use IPv4.
- **Docker Desktop, rootless Docker, a reverse proxy on the LAN and tunnels on the same host**
  (cloudflared, Tailscale Serve or Funnel, ngrok) replace every client address with a private
  or loopback one. Behind them the guard lets everyone through, so the proxy, tunnel or
  firewall has to restrict access.
- **VPS and hosting**: some providers put many customers into one on-link /64; the guard then
  admits those neighbours. Restrict access with a firewall or proxy there.
- **DHCPv6-only networks**: stateful DHCPv6 addresses appear as `/128`, so the server learns
  no LAN prefix and clients with a global IPv6 address get `403`; use IPv4 or a unique local
  address.
- **`100.64.0.0/10`** is admitted for Tailscale. It also admits carrier-grade-NAT neighbours
  at the ISP when the router's WAN address lies in that range and the port is forwarded,
  devices shared into your tailnet from other tailnets, and cloud or Kubernetes networks
  that use the range.

Set `MIVO_SERVER_LAN_ONLY=false` only when another layer controls who can reach the port.

### Configure through environment or `/config`, not command flags

Change settings such as the port with `MIVO_*` variables (`-e MIVO_SERVER_PORT=9000`) or in
`/config/config.yaml`, and keep the config directory at `/config` (`MIVO_CONFIG_DIR`). Do not
append server flags such as `--config-dir` to `docker run`: the `HEALTHCHECK` runs as a
separate process that reads only the environment and `/config`, so it would probe the wrong
port or configuration and report the container as unhealthy.

## User and file ownership

The container never runs as root. The default user is `1000:1000`.

Bind mounts keep the owner of the host directory. Run the container as the user that owns
the mounted directories. On a NAS this is not optional: folders created in File Station
belong to your NAS user, so with the default `1000:1000` the server cannot write `/config`
and stops at once (the log ends with `permission denied`, see below). Find the IDs with
`id <user>` on the NAS:

- Synology: the first user is typically `1026`, group `users` is `100`.
- QNAP: a created user is typically `1000`, group `everyone` is `100`, so `1000:100`.
  The built-in `admin` account has UID `0`; create a dedicated user instead.
- Never run the server with UID `0`: it must not run as root.

```
docker run --user 1026:100 ...
```

Synology (Container Manager project), one library from the `video` share:

```yaml
services:
  mivo:
    image: docker.io/magpeek/mivo-media-server:0.1.0-alpha.5
    user: "1026:100"
    stop_grace_period: 20s
    ports:
      - "0.0.0.0:8460:8460"
    environment:
      TZ: Europe/Berlin
      MIVO_TRANSCODE_MAX_SESSIONS: "1"
    volumes:
      - /volume1/docker/mivo/config:/config
      - /volume1/video:/media:ro
      - /volume1/docker/mivo/transcode:/transcode
```

QNAP (Container Station application), two libraries from the `Multimedia` share:

```yaml
services:
  mivo:
    image: docker.io/magpeek/mivo-media-server:0.1.0-alpha.5
    user: "1000:100"
    stop_grace_period: 20s
    ports:
      - "0.0.0.0:8460:8460"
    environment:
      TZ: Europe/Berlin
      MIVO_LIBRARIES: /media/movies:/media/series
    volumes:
      - /share/Container/mivo/config:/config
      - /share/Multimedia/Movies:/media/movies:ro
      - /share/Multimedia/Series:/media/series:ro
      - /share/Container/mivo/transcode:/transcode
```

Put `transcode` on a volume with enough free space (see Transcoding); the user must be able to
read every library folder and write `config` and `transcode`.

Named and anonymous volumes: Docker copies the owner of the image directories (`1000:1000`)
into a volume only when it creates the empty volume. They work with the default user, but
with `--user` / `user:` set to another ID the server cannot write them. Use bind mounts in
that case, or `chown` the volume to that user once before the first start (a one-off root
container that only changes ownership, not the server):
`docker run --rm -v mivo-config:/config --user 0 --entrypoint chown docker.io/magpeek/mivo-media-server:0.1.0-alpha.5 -R 1026:100 /config`.
Note that Docker re-initialises a volume from the image while it is still empty, so the
`chown` only sticks once the volume holds a file; bind mounts avoid the problem.

`/config` and `/transcode` must be writable by that user and `/media` readable. If they are
not, the server stops at startup with a message that names the directory, for example
`mivo: load configuration: read /config/config.yaml: open /config/config.yaml: permission denied`.
An unwritable `/transcode` does not stop the start: the log warns `leftover playback session
directories could not be deleted … open /transcode: permission denied` and remux and
transcode sessions fail later. An unreadable `/media` ends the scan with
`library scan finished … state=failed code=root_unavailable`. (All three messages verified
with the `0.1.0-alpha.5` image.) The server creates its directories with mode `0750`, so the
user's group can read them.

`PUID`/`PGID` variables with a privilege drop in an entrypoint come in milestone 9.
