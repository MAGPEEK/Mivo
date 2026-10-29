# Mivo

> Working title. The final name is decided before the first public release.

Mivo is an open-source media server that plays practically any container format.
It runs on Linux (Synology DSM, QNAP and other NAS via Docker) and Windows, and
adapts every stream to the client: direct play, remux or transcode.

**Status:** pre-alpha, under active development. Not usable yet.

## Goals

- Every container FFmpeg can read: MKV, MP4, AVI, TS/M2TS, WebM, ISO, BDMV, …
- Movies, shows, music, photos, audiobooks, disc images
- Plex-like web UI, DLNA, Chromecast, LG webOS TV app
- Lightweight single Go binary, works on weak ARM NAS
- Local accounts only, no cloud dependency

## License

GPL-3.0-or-later. See [LICENSE](LICENSE).
