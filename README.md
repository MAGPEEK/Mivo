# Mivo

> Working title. The final name is decided before the first public release.

An open-source media server that plays practically any container format. It runs on Linux
(Synology DSM, QNAP and other NAS via Docker) and Windows, and adapts every stream to the
client: direct play, remux or transcode.

**Status:** alpha (v0.1-alpha), under active development. Expect missing features and breaking
changes; keep a copy of anything you care about.

## What v0.1-alpha can do

- **Scan** the libraries you configure (folders of video files) with `ffprobe`, at every start
  and on demand from the web app.
- **Browse** them in the web app: libraries in the sidebar, a paged grid of items, and a detail
  page with the technical facts (container, codecs, resolution, HDR, audio and subtitle
  tracks).
- **Play** every video container of the generated test corpus in the browser: MKV, MP4, M4V,
  MOV, WebM, AVI, MPEG-TS (TS, M2TS, MTS), MPEG-PS (MPG, VOB), MXF, FLV, 3GP, OGV, WMV and
  RealMedia (RM, RMVB; RealVideo 1/2 only, RealVideo 4 is untested).
- **Choose the cheapest way per file and browser**: Direct Play sends the original file,
  Remux (shown as "Direct Stream" in the player) repackages the streams into HLS without
  re-encoding, Transcode re-encodes to H.264/AAC (at most 1080p) on the CPU. The player's
  stream info overlay shows which mode was chosen and why.
- **Seek** anywhere in the file; a seek outside the part produced so far restarts FFmpeg there
  (a remux at the keyframe before the target).

## What it cannot do yet

- **No login or user accounts** (milestone 7): everyone in your local network who can reach
  the port can use it.
- **No subtitles** in the player and no subtitle burn-in (milestone 4).
- **No hardware acceleration**: transcoding uses the CPU only (milestone 4).
- **No metadata**: titles come from file names; no posters, descriptions or TMDB lookups
  (milestone 6). Every library is a flat list of videos: no seasons and episodes, no music
  or photo libraries yet.
- Libraries are set in the configuration, not in the web app, and changes need a restart.

## Local network only

There is **no authentication until milestone 7**. By default the server refuses requests from
outside the local network (`server.lan_only`). What counts as local, and how Docker, proxies
and tunnels change it, is explained in the section "Local network only" of
[packaging/docker/README.md](packaging/docker/README.md). Do not forward the port from your router,
do not put it behind a public reverse proxy or tunnel, and do not turn the guard off unless
another layer controls who can reach the port.

## Run it

- **Docker / NAS**: build and run the image as described in
  [packaging/docker/README.md](packaging/docker/README.md) (volumes, user IDs, libraries,
  transcoding, Synology and QNAP examples). Mount your videos at `/media`; the web app is on
  port 8460.
- **From source** (Windows or Linux): [docs/development.md](docs/development.md) covers Go,
  Node.js, FFmpeg, configuring libraries and tools, and running the server.

## Goals

- Every container FFmpeg can read: MKV, MP4, AVI, TS/M2TS, WebM, ISO, BDMV, …
- Movies, shows, music, photos, audiobooks, disc images
- Plex-like web UI, DLNA, Chromecast, LG webOS TV app
- Lightweight single Go binary, works on weak ARM NAS
- Local accounts only, no cloud dependency

## Development

Setting up, building, testing and running the server and the web app on Windows
(PowerShell) or Linux is described in [docs/development.md](docs/development.md).
Architecture decisions live in [docs/adr](docs/adr/).

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
