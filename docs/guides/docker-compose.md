# Running the server with Docker Compose

Docker Compose keeps the whole configuration of the container in one file, so updates and
changes are an edit plus `docker compose up -d`. This page assumes you have read "Local
network only" and "Before you start" in [docker.md](docker.md): the warning about exposing
the server, the three folders and the user ids apply unchanged. The settings of the image
are explained in [packaging/docker/README.md](../../packaging/docker/README.md).

The ready-to-use file is
[packaging/docker/docker-compose.yml](../../packaging/docker/docker-compose.yml). It pins
the image `docker.io/magpeek/mivo-media-server:0.1.0-alpha.2`, publishes port 8460 for IPv4,
mounts `/media` read-only, sets `restart: unless-stopped` and `stop_grace_period: 20s`
(Compose's default of 10 s would kill the server in the middle of its graceful shutdown),
and inherits the image's health check. You need to edit the three host paths and, on a NAS,
the `user:` line.

## Plain Docker Compose (Linux server, Docker Desktop)

1. Create a folder for the project and the three data folders, for example:

   ```
   mkdir -p ~/mivo /srv/mivo/config /srv/mivo/transcode
   ```

2. Download the compose file into the project folder:

   ```
   curl -fsSLo ~/mivo/docker-compose.yml \
     https://raw.githubusercontent.com/MAGPEEK/Mivo/main/packaging/docker/docker-compose.yml
   ```

3. Edit it: replace `/path/to/config`, `/path/to/videos` and `/path/to/transcode` with your
   folders, set `TZ`, and uncomment `user:` with the owner of the folders if that is not
   `1000:1000` (`id <user>`). Check the result:

   ```
   cd ~/mivo
   docker compose config      # prints the resolved file or an error
   ```

4. Start, check and open:

   ```
   docker compose up -d
   docker compose ps          # STATUS "Up … (healthy)" after a few seconds (about 30 s on Engine < 25)
   docker compose logs -f     # "http server listening", then "library scan finished"
   ```

   Open `http://<host>:8460`.

Day-to-day commands, all from the folder with the file:

| Task | Command |
|---|---|
| stop / start | `docker compose stop`, `docker compose start` |
| apply an edited file | `docker compose up -d` (recreates only what changed) |
| update to a new version | change the tag in the file, then `docker compose pull && docker compose up -d` |
| logs | `docker compose logs -f --tail 100` |
| remove the container (data in `/config` stays) | `docker compose down` |

`docker compose up -d` with an unchanged file does nothing, so running it twice is safe.

Several libraries: mount each folder below `/media` and set `MIVO_LIBRARIES` (the comments
in the file show both lines). Names of your own or the `shows` kind need `config.yaml` and
the `entrypoint:` workaround from [docker.md](docker.md), section 6:

```yaml
    entrypoint: ["/usr/bin/env", "-u", "MIVO_LIBRARIES", "/usr/local/bin/mivo"]
```

## Synology Container Manager (DSM 7.2, for example DS925+)

Container Manager runs compose files as *Projects*. The steps below were written for a
DS925+ (AMD Ryzen V1500B, x86-64, DSM 7.2) and apply to every model that offers Container
Manager (no 32-bit ARM model does; for those and for an install without Docker use the
native package, [packaging/synology/README.md](../../packaging/synology/README.md)).

> **Unverified UI labels.** The menu names, buttons and tabs below are taken from Synology's
> DSM 7.2 Container Manager documentation and have not yet been verified on a device. Labels
> marked *(verify)* are the most likely to differ; the compose file and the commands are
> verified.

### Quirks to know first

- **Folders belong to your DSM user, not to `1000`.** Folders created in File Station are
  owned by the user who created them, usually the first DSM user with uid `1026` and group
  `users` (gid `100`). The image runs as `1000:1000`, cannot write such a folder, and the
  container then stops immediately with `permission denied` in the log. So the `user:` line
  in the compose file is **mandatory** on Synology, not optional.
- **Paths are `/volume1/<shared folder>/…`** (`/volume2/…` for a second volume). You type the
  host paths of the volumes into the YAML; the folder picker of the project only chooses
  where the compose file is stored *(verify)*. Shared folders show their volume in Control
  Panel > Shared Folder.
- **`stop_grace_period: 20s`**: Docker Compose honours it, and Container Manager runs
  projects with Docker Compose, so the server gets 20 s to shut down when you stop the
  project. Whether DSM's own shutdown of the NAS waits as long is not verified.
- **Docker version and health status.** Container Manager on DSM 7.2 ships its own Docker
  Engine, which may be older than 25. Older engines ignore the image's fast start probe
  (`--start-interval`), so *Healthy* appears only after about 30 s instead of a few seconds.
  Check the version over SSH with `sudo docker version` (step 1 below).
- **Do not publish the server** through DSM's reverse proxy or the web portal settings of the
  project: behind them every client looks local and the LAN-only guard lets everyone in.
  QuickConnect is not expected to reach the port at all (ADR 0026); do not try to publish the
  server there either. See [docker.md](docker.md), "Local network only". If the DSM firewall
  is on, allow TCP 8460 for your LAN in Control Panel > Security > Firewall.

### Walkthrough

1. **Find your ids and the Docker version.** Enable SSH once (Control Panel > Terminal &
   SNMP > Enable SSH service), log in with your DSM user (`ssh <user>@<nas>`) and run:

   ```
   id
   sudo docker version --format '{{.Server.Version}}'
   ```

   `id` prints for example `uid=1026(stefan) gid=100(users) groups=100(users),101(administrators)`;
   note `uid` and `gid`, here `user: "1026:100"`. The second line prints the engine version
   (see "Quirks"). Disable SSH again afterwards if you do not use it. Without SSH, use
   `1026:100`: the first user created on a DSM system normally has uid `1026`, and group
   `users` is `100`. If the container then stops with `permission denied`, the ids are wrong
   and you need `id` after all.

2. **Create the folders** in File Station, inside the shared folder `docker` that Container
   Manager created (or any shared folder on a local volume):

   - `docker/mivo/config`
   - `docker/mivo/transcode`

   Your videos stay where they are, for example in the shared folder `video`. Make sure
   your user has read permission on it (Control Panel > Shared Folder > Edit > Permissions).
   Do not use a shared folder that lives on an external USB disk or a remote mount for
   `config`.

3. **Create the project.** Open Container Manager > Project > Create *(verify)*:

   - Project name: `mivo`
   - Path: pick `/docker/mivo` with the folder picker (this is where the compose file is
     stored, not a data folder).
   - Source: *Create docker-compose.yml* *(verify)*, then paste the content of
     [packaging/docker/docker-compose.yml](../../packaging/docker/docker-compose.yml) and
     edit it:

   ```yaml
   name: mivo

   services:
     mivo:
       image: docker.io/magpeek/mivo-media-server:0.1.0-alpha.2
       container_name: mivo
       restart: unless-stopped
       stop_grace_period: 20s
       user: "1026:100"
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

   Replace `1026:100` with your ids and the paths with yours. Click Next; leave the web
   portal settings off *(verify)* (see "Quirks"); click Done. Container Manager pulls the
   image and starts the container.

4. **Check it.** Container Manager > Container *(verify)* shows `mivo` as running; the health
   state changes from starting to healthy after a few seconds, or after about 30 s on an
   engine older than 25 (where Container Manager shows the health is not verified; over SSH,
   `sudo docker ps --filter name=mivo` shows `(healthy)` in the STATUS column). Open the
   container's log *(verify: Details > Log)*: the last lines should be
   `http server listening … lan_only=true` and `library scan finished … failed=0`.

5. **Open** `http://<NAS address>:8460` from a device in your LAN. DSM asks nothing; the web
   app has no login yet.

### Day to day on Synology

All menu names in this list are *(verify)*.

- **Edit settings**: stop the project, edit its YAML, save and build (or start) it again. An
  edit without recreating the container changes nothing.
- **Update**: change the tag in the YAML, save, build; Container Manager pulls the new image
  and recreates the container. The database in `config/data` is kept and migrated. Before an
  update, copy `docker/mivo/config` with File Station or Hyper Backup (downgrades are not
  supported).
- **Logs**: the container's log in Container Manager, or `sudo docker logs mivo` over SSH.
- **Autostart**: `restart: unless-stopped` in the YAML makes Docker start the container again
  after a reboot, unless you stopped it yourself.
- **More libraries**: add volume lines below `/media` and a `MIVO_LIBRARIES` line, as in the
  comments of the compose file. Every shared folder you mount must be readable by the user
  in `user:`.

### If it does not start on Synology

The container is stopped or restarts in a loop, and its log ends with:

```
mivo: load configuration: read /config/config.yaml: open /config/config.yaml: permission denied
```

The `user:` line is missing or names the wrong ids. Stop the project, fix `user:` and build
it again. Nothing is written into `config` or `transcode` when the start fails this way, so
no cleanup is needed. If an earlier start did run with other ids, the folders `config/data`,
`config/cache` and `transcode/sessions` belong to those ids; delete them in File Station
(they hold no data worth keeping before the first successful start) and start again.

Other messages are explained in [docker.md](docker.md), "Troubleshooting": a scan with
`code=root_unavailable` means the user in `user:` cannot read the video share; a warning
about `/transcode` with `permission denied` means the transcode folder belongs to someone
else.

## QNAP Container Station

Container Station (QTS 5 / QuTS hero) runs compose files as *Applications*.

> **Unverified UI labels.** The menu names, buttons and tabs below are taken from QNAP's
> Container Station 3 documentation and have not yet been verified on a device. Labels marked
> *(verify)* are the most likely to differ; the compose file is verified.

### Quirks to know first

- **Paths are `/share/<shared folder>/…`**, for example `/share/Multimedia` or
  `/share/Container` (the folder Container Station created). On multi-volume models the
  real path is `/share/CACHEDEV1_DATA/<folder>`; `/share/<folder>` is a symlink that works
  in compose files.
- **User ids**: a user you create in QTS typically gets uid `1000` and the group `everyone`
  has gid `100`, so `user: "1000:100"` is typical *(verify with `id <user>` over SSH, enabled
  in Control Panel > Network & File Services > Telnet / SSH)*. The built-in `admin` is uid
  `0` and must not be used.
- **`stop_grace_period`**: Docker Compose honours it; Container Station runs applications
  with Docker Compose.
- Do not expose the server through myQNAPcloud Link, the QNAP reverse proxy or port
  forwarding on the router.

### Walkthrough

1. Create a user for the server (Control Panel > Users) or use your own; grant it read
   access to the multimedia shares and read/write to the `Container` share (Shared Folders >
   Edit Shared Folder Permission *(verify)*).
2. Create the folders `Container/mivo/config` and `Container/mivo/transcode` in File Station.
3. Container Station > Applications > Create *(verify)*. Name `mivo`, then paste and edit
   the compose file:

   ```yaml
   name: mivo

   services:
     mivo:
       image: docker.io/magpeek/mivo-media-server:0.1.0-alpha.2
       container_name: mivo
       restart: unless-stopped
       stop_grace_period: 20s
       user: "1000:100"
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

   This example mounts two libraries; for a single folder mount it at `/media` and drop the
   `MIVO_LIBRARIES` line. Click Create; Container Station pulls and starts.
4. Open the application's container *(verify)*: it shows the state and the health and has a
   log view. Open `http://<NAS address>:8460`.

Updates: edit the application's YAML *(verify)*, change the tag and apply; Container Station
pulls and recreates. Back up `Container/mivo/config` before that.

## Validating a compose file

Before you paste a file into a NAS UI, you can check it on any machine with Docker:

```
docker compose -f docker-compose.yml config
```

It prints the resolved configuration or names the line with the error. The file in this
repository was validated and run against the published `0.1.0-alpha.2` image with the test
corpus; the container reported `healthy` after about 4 s and `/api/v1/system/info` answered
with `{"apiVersion":"1","productName":"Mivo","version":"0.1.0-alpha.2",…}`.
