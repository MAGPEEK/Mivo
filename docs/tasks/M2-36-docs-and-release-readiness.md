# M2-36 Documentation and release readiness for v0.1-alpha
Owner: mivo-backend
Depends on: all other M2 tasks
Files/areas: `docs/development.md`, `README.md`, `CLAUDE.md` ("Commands" section only, needs the
user's approval like M1-22), `docs/tasks/BACKLOG.md`, `docs/reports/M2-release-readiness.md`

## Goal
A new contributor and a NAS user can set up libraries, FFmpeg and playback from the docs, and the
milestone ends with one recorded run of every gate on a clean checkout, ready for the release
decision (OD-9).

## Acceptance criteria
- [ ] `docs/development.md`: libraries and tools configuration (keys, env, examples for PowerShell
      and bash), test corpus incl. long clips and `-only` lists, the corpus integration test and
      `-short`, the playback smoke script, E2E browser projects (bundled Chromium and branded
      channel), the run-time impact of FFmpeg tests.
- [ ] BACKLOG rows "docs coverage without cgo" and "docs nits" are done.
- [ ] `README.md`: what v0.1-alpha can do (scan, browse, play every container of the corpus,
      Direct Play / Remux / Transcode with overlay), what it cannot yet (login, subtitles, HW
      acceleration, metadata), the LAN-only warning (no authentication until milestone 7).
- [ ] `CLAUDE.md` commands: smoke script and corpus generation lines (after approval).
- [ ] `docs/tasks/BACKLOG.md` lists every non-blocking finding from M2 reviews not yet done.
- [ ] `docs/reports/M2-release-readiness.md`: on a clean clone, the full gate list of
      `docs/development.md` ("Full gates before hand-in"), `go run ./scripts/gen-testmedia`,
      the corpus integration test, `pnpm -C web e2e` (both projects) and the Docker smoke run,
      each with command and result; list of open decisions still unanswered.
- [ ] Every relative link in the changed Markdown files resolves (tester checks with a grep of
      `](` targets against the tree).
- [ ] Gates: docs-only change, so the Go and web gates must still pass unchanged
      (`golangci-lint run`, `go test ./...`, `pnpm -C web lint`).

## Notes
- The release itself (version tag, image push) is done by `mivo-release` only after PASS +
  APPROVE and the user's explicit approval (OD-9).
- Keep the product name out of prose where the brand rule applies (ADR 0002 rename checklist).

## Orchestrator note (from M2-06)
- `docs/development.md` "Test media corpus": `-only` now takes a comma-separated glob list
  (e.g. `'video-mp4-*,video-webm-vp9-opus'`), and kept files are labelled `kept` in progress and
  summary. Document both.
- (from M2-12) User docs: absolute symlinks and Windows junctions inside a library are skipped
  (only relative links that stay inside the root are followed); use relative links or add the
  target as its own library.
- (from M2-13) User docs: keep the data directory (`/config` in Docker) on local storage; file
  locks on SMB/NFS shares may not be honoured.
- (from M2-08) ADR 0024 "Notes (M2)": requests with a body get a 30 s read deadline
  (`server.BodyReadTimeout`, set in the server middleware); bodiless requests (stream GETs) never do.
- (from M2-16) `internal/adapters/ffmpeg/doc.go` still says "later tasks add capability detection";
  update after M2-16/M2-17. ADR 0023 "Notes (M2-16)": detection is all-or-nothing under one 30 s
  timeout (failure → direct play only), experimental encoders (flag X) do not count, version string
  printable ASCII cut to 64 bytes; FFmpeg 7.1.5 marks native opus/vorbis/truehd/dca as experimental.
- (from M2-21) ADR 0024 "Notes (M2-21)": restart keeps 1 s before the end, recovery rules
  (404/410 on session requests, fatal hls.js network errors; 503 stream_not_ready is not a
  recovery case), unknown modes/codes → fallback keys and `textMuted`, hardware encoders detected
  by FFmpeg suffix.
- (from M2-20 review) ADR 0024 "Notes (M2-20)": stream URLs are built once when a session is
  created; M7 (time-limited/owner-signed URLs) must build them on read and changes the URLBuilder
  port (owner, expiry). Overall session cap incl. direct play (MaxSessions).
- (from M2-17) ADR 0024 "Notes (M2-17)": HLS output paths are built with forward slashes (a
  backslash path makes the hls muxer write init.mp4 into FFmpeg's working directory); every seek
  with copied video uses `-copypriorss 0` and StartPosition must be the KeyframeAtOrBefore result;
  every AAC copy uses `-bsf:a aac_adtstoasc` (ADTS from MPEG-TS); `%` in the transcode path is
  rejected (FFmpeg reads it as a template).
- (from M2-15) ADR 0022 "Notes (M2-15)": the primary file of an item is its first file by relative
  path; the items cursor is base64url(id NUL sortTitle), strictly decoded (opaque to clients);
  undecodable stored rows return a plain error, never ErrInvalid; RemoveFiles ignores unknown or
  other-library ids.
