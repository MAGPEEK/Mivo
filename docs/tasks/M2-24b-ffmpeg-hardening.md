# M2-24b FFmpeg hardening: pinned demuxers and orphan processes
Owner: mivo-media
Depends on: M2-24 (committed), M2-11, M2-14
Files/areas: `internal/adapters/ffmpeg/**` (+ tests), notes in ADR 0022 / ADR 0024

## Goal
A hostile file in a library cannot make FFmpeg or ffprobe read other local files, and FFmpeg
children do not outlive a crashed server (follow-ups from the M2-24 review).

## Acceptance criteria
- [ ] Every ffprobe and ffmpeg invocation of the package restricts input demuxers so that
      playlist-like demuxers (hls, concat, dash, image2 sequences, ffconcat, tee and similar) can
      never open other files: either `-format_whitelist` with the containers the decision engine
      and the scanner accept, or `-f <format>` pinned from the probe result for the producer and
      the keyframe lookup – choose, justify in ADR 0022 Notes, and keep the corpus fully probe- and
      playable. `-protocol_whitelist file` stays.
- [ ] Test: a file named `evil.mkv` whose content is an HLS playlist (and one that is an ffconcat
      list) referencing another local file is refused by probe, keyframe lookup and producer; the
      referenced file is never read (assert via a canary file's access or via FFmpeg's error).
- [ ] FFmpeg children die with the server: Linux `SysProcAttr.Pdeathsig = SIGKILL` (build-tagged
      file), Windows a Job Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (uses golang.org/x/sys,
      no new module). Test on both systems where feasible (Linux in Docker: kill the parent process
      of a helper and assert the child is gone).
- [ ] `ErrorOutput` doc comment (process.go) says the tail may be logged (the producer logs it once
      at warn per ADR 0024) but is never returned to clients, and that it can contain absolute media
      paths and bytes from the file.
- [ ] Windows: a corpus test with a ~1 ms playlist poll interval through many rewrites proves
      FFmpeg's rename-over works while Ready polls (clean Done, `#EXT-X-ENDLIST`).
- [ ] Nits from the M2-24 review: no `exit_code` 0 for failures that are not exits (omit it or a
      named `unknownExitCode = -1`); name the 5 s grace in `stopTimeout` (`stopKillGrace`); the
      header-only Ready test waits for `index.m3u8` before starting its not-ready deadline.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` (Docker) + corpus
      integration tests on Windows (FFmpeg 9.x) and Linux (FFmpeg 7.1.5) + coverage gate.
