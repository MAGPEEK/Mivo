# M2-32 Composition root: wire scanning and playback
Owner: mivo-backend
Depends on: M2-04, M2-05, M2-10, M2-11, M2-12, M2-13, M2-15, M2-16, M2-18, M2-19, M2-24, M2-25,
M2-26, M2-28, M2-29, M2-31 (if OD-10 is accepted)
Files/areas: `cmd/mivo/**`, `internal/adapters/http/restapi/api.go` (make services mandatory,
remove `not_implemented`)

## Goal
`go run ./cmd/mivo` with a library configured scans it, serves the catalog and plays every corpus
video – the walking skeleton of milestone 2 runs end to end (ADR 0022–0024).

## Acceptance criteria
- [ ] Startup order: config → logger → directories → instance lock → database + migrate → FFmpeg
      tools (`exec.LookPath` for bare names) and capability detection (logged: version, H.264/AAC
      encoders, or a warning "only direct play available") → workspace purge → library sync from
      config → services (library, scan, playback with limits, capabilities, policy, URL builder)
      → HTTP (restapi with all services and the stream mounts, quiet prefix
      `/api/v1/playback/sessions/`, LAN guard per OD-10) → startup scan per OD-3.
- [ ] Missing ffprobe: server starts; scans fail with a logged, clear error. Missing ffmpeg:
      server starts; sessions needing HLS fail with `transcode_unavailable` / `no_compatible_format`.
- [ ] Shutdown order: HTTP server stops → playback `Close` (all FFmpeg killed, directories removed)
      → scan `Close` → database close → lock release; each step bounded and logged; a second
      Ctrl+C still behaves as in milestone 1.
- [ ] `restapi.NewAPI` rejects nil services (`ErrInvalid`); the internal `not_implemented` code and
      stub branches are gone.
- [ ] `cmd/mivo` tests: start with a temp library holding two corpus files (skip without
      FFmpeg/corpus) → `GET /api/v1/libraries` lists it → scan finishes → items listed → a session
      for the MP4 (direct play) returns a working `streamUrl` (206 on a range request) → a session
      for the MKV (remux) serves a playlist → shutdown leaves `<transcode>/sessions` empty.
- [ ] Manual check documented in the REPORT (PowerShell): `$env:MIVO_LIBRARIES =
      "$PWD\testdata\media"`, isolated dirs as in `docs/development.md`, `go run -tags webui
      ./cmd/mivo`, browse to the library, play one file per mode in Chrome or Edge.
- [ ] Gates: all Go gates incl. `-tags webui` variants, `pnpm -C web e2e` (existing specs),
      `docker build -f packaging/docker/Dockerfile -t mivo:dev .`.

## Notes
- Keep `serve.go` functions ≤ 50 lines: split wiring per area (`wire_library.go`,
  `wire_playback.go`); `cmd` holds no logic beyond wiring and ordering.
- Convert `config.LibraryConfig` to `library.Definition` here (platform must not import domain).
- `app` gets values (limits, capabilities, policy height from OD-5) through constructor options,
  never the config struct (ADR 0003).

## Orchestrator note (from M2-04 and M2-05 reviews)
- Convert each `config.LibraryConfig` with `library.ParseKind` and validate it with
  `library.Definition.Validate`; wrap failures as `config: libraries[<i>] (<path>): …` so the
  message names the entry (the config package cannot check domain limits such as the 128-rune
  name or NUL bytes). Test that every config kind parses.
- Resolve `tools.ffmpeg` / `tools.ffprobe` once at startup with `exec.LookPath` (bare names) or a
  stat check (paths): must exist and be a regular file; log the resolved absolute paths. Fail
  startup with a clear message otherwise.
- Name `transcodedir.ErrSessionsNotPlainDirectory` in the startup message if the transcode
  workspace refuses `<transcode>/sessions` (symlink/junction/file).
- (from M2-11 review) On Windows, reject `tools.ffmpeg` / `tools.ffprobe` that resolve (after
  `LookPath` with PATHEXT) to a `.bat` or `.cmd` file: `cmd.exe` re-parses such command lines and
  a media file name like `x&\host\s\evil&.mkv` could inject commands. Test it.
