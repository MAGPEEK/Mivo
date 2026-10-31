# Docker image

The container image of the media server (ADR 0014). It holds one static binary with the
embedded web app (ADR 0020) and Debian's FFmpeg (interim build, ADR 0015).

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
and `web/` (without `node_modules`, build output, coverage and Playwright output).

## Run

```
docker run -d --name mivo -p 8460:8460 --stop-timeout 20 \
  -v /path/to/config:/config -v /path/to/media:/media:ro -v /path/to/transcode:/transcode \
  mivo:dev
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
| `/media` | media libraries, read only is enough |
| `/transcode` | temporary transcoding output |
| port `8460` | web app and REST API |
| `MIVO_*` | every setting of ADR 0018, e.g. `MIVO_LOG_LEVEL=debug` |
| `TZ` | time zone, e.g. `TZ=Europe/Berlin` (tzdata is installed) |

`HEALTHCHECK` runs `mivo healthcheck`, which GETs `/api/v1/health` on the configured port.

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
the mounted directories. Find the IDs with `id <user>` on the NAS:

- Synology: the first user is typically `1026`, group `users` is `100`.
- QNAP: a created user is typically `1000`, group `everyone` is `100`, so `1000:100`.
  The built-in `admin` account has UID `0`; create a dedicated user instead.
- Never run the server with UID `0`: it must not run as root.

```
docker run --user 1026:100 ...
```

```yaml
services:
  mivo:
    image: mivo:dev
    user: "1026:100"
    stop_grace_period: 20s
    volumes:
      - /volume1/docker/mivo/config:/config
      - /volume1/video:/media:ro
      - /volume1/docker/mivo/transcode:/transcode
```

Named and anonymous volumes: Docker copies the owner of the image directories (`1000:1000`)
into a volume only when it creates the empty volume. They work with the default user, but
with `--user` / `user:` set to another ID the server cannot write them. Use bind mounts in
that case, or `chown` the volume to that user once before the first start (a one-off root
container that only changes ownership, not the server):
`docker run --rm -v mivo-config:/config --user 0 --entrypoint chown mivo:dev -R 1026:100 /config`.

`/config` and `/transcode` must be writable by that user and `/media` readable. If they are
not, the server stops at startup with a message that names the directory, for example
`mivo: load configuration: read /config/config.yaml: open /config/config.yaml: permission denied`.
The server creates its directories with mode `0750`, so the user's group can read them.

`PUID`/`PGID` variables with a privilege drop in an entrypoint come in milestone 9.
