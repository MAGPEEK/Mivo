# M2-35 Playback smoke test and Docker verification
Owner: mivo-media
Depends on: M2-32, M2-33
Files/areas: `scripts/playback-smoke/**` (new), `packaging/docker/Dockerfile` (library default
per OD-1 only), `packaging/docker/README.md`

## Goal
The same expectations that `go test` checks on the dev machine's FFmpeg 9 are checked against the
real Docker image with its FFmpeg 7.1 – the release gate for v0.1-alpha (ADR 0025).

## Acceptance criteria
- [ ] `go run ./scripts/playback-smoke -base <url> -manifest <path> [-profile chromium-desktop]`
      triggers a scan, waits for it, then for every generated video case of the manifest finds the
      item by file name, creates a session, checks mode and reasons against
      `corpustest` expectations, downloads the stream (direct: range request → 206; HLS: playlist
      until `#EXT-X-ENDLIST`, init, all segments), probes the joined output with the local ffprobe,
      deletes the session, and prints one line per case plus a summary; exit code 1 on any
      mismatch.
- [ ] Unit tests for the HTTP and playlist parts against `httptest` servers; coverage ≥ 80 % for
      the package (excluding `main`).
- [ ] Docker (Dockerfile per OD-1: `ENV MIVO_LIBRARIES=/media` if accepted): build `mivo:dev`, run
      with `-v <repo>/testdata/media:/media:ro`, port mapped, default user 1000 → container
      becomes `healthy`; the smoke run against it passes for every case; the REPORT quotes the
      image's `ffmpeg -version` line and the summary.
- [ ] After the smoke run `docker exec <c> ls /transcode/sessions` is empty; nothing was written to
      `/media` (read-only mount, no errors in the log).
- [ ] `packaging/docker/README.md`: libraries (`MIVO_LIBRARIES`, YAML alternative), transcode
      limit, `/transcode` space note (no throttling until milestone 4), FFmpeg build note
      (Debian interim, ADR 0015), Synology/QNAP examples updated.
- [ ] Gates: lint + layer rules + `go test -race ./scripts/...`; `docker build` succeeds.

## Notes
- The script is a client of the public API only (no internal imports except the generated API
  types and `corpustest` expectations).
- On a Linux host, corpus files are `0640`: the container user must be able to read them
  (document `--user` or a readable copy).
- Publishing the image is **not** part of this task (mivo-release, with the user's approval).
