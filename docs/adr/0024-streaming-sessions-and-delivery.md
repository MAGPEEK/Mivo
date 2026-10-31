# 0024 Streaming sessions and delivery (Direct Play, HLS remux and transcode)

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-30
- Refines: tech stack row "Streaming" in `docs/plan.md`, ADR 0015 (FFmpeg processes),
  ADR 0017 (API contract)

## Context

The plan prescribes HLS with fMP4 segments, seeking by restarting FFmpeg at a keyframe, and a
transcode cache. Milestone 2 runs without login, but milestone 7 adds accounts and milestone 7b
remote access with signed, time-limited stream URLs. The stream API must allow that without a
breaking change. Throttling, bandwidth tiers and hardware encoders arrive in milestone 4.

## Decision

**Sessions**
- Every playback is a **session**, created with `POST /api/v1/playback/sessions` (media file ID,
  client profile, optional start position, optional audio stream index, optional
  `replacesSessionId`). The response carries the plan (mode, reasons, warnings, per-stream
  actions, encoder), the actual start position and an opaque `streamUrl`.
- Sessions live in memory in `app/playback` (no table in milestone 2). Session IDs are random
  UUIDs (122 bits), so a stream URL cannot be guessed.
- States: `starting` → `ready` → `ended`, or `failed`. `GET …/sessions/{id}` returns the
  session; `DELETE` stops it (idempotent, 204).
- **Idle timeout**: a session whose files were not requested for 2 minutes (named constant) is
  stopped: FFmpeg is killed and its directory removed. hls.js reloads an unfinished playlist every
  target duration, which keeps a playing or paused session alive. A client that gets 404 for a
  stopped session creates a new one at its current position.
- **Limits** (OD-4 A, decided 2026-10-30): at most `transcode.max_sessions` concurrent sessions
  that re-encode a stream, plus a fixed safety cap of 8 sessions running FFmpeg at all (remux
  included). A request beyond a limit fails with 503,
  problem code `session_limit_reached` and `Retry-After`. `replacesSessionId` stops the old
  session before the limit is checked, so seeking never trips the limit.
- FFmpeg runs under a context owned by the playback service, never under the HTTP request
  context, and is killed on stop, idle timeout and server shutdown. The service waits for the
  processes and removes their directories before the server exits.

**Delivery**
- `direct_play`: `GET|HEAD /api/v1/playback/sessions/{id}/file` serves the original file with
  `http.ServeContent` (Range, `If-Range`, 206/416), opened through `os.Root` of the library root
  (ADR 0022), `Content-Type` from the ADR 0023 table.
- `remux` and `transcode`: FFmpeg's `hls` muxer writes into `<transcode>/sessions/<session id>/`:
  `index.m3u8` (playlist type `EVENT`), `init.mp4` and `seg-00000.m4s`, … with a 6 s target
  duration, `independent_segments` and `temp_file` (FFmpeg writes to a temporary name and
  renames, so the server never serves half a file). When video is transcoded, keyframes are
  forced at segment boundaries. Copied HEVC is tagged `hvc1` (Safari).
- `GET /api/v1/playback/sessions/{id}/hls/{fileName}` serves those files. `fileName` must match
  `index.m3u8`, `init.mp4` or `seg-[0-9]{5}.m4s`; files are opened through `os.Root` of the
  session directory. A playlist request waits up to 20 s (named constant) for the first segment,
  then answers 503 `stream_not_ready` with `Retry-After` (every HLS file waits, and the route
  supports `HEAD` and Range; Notes M2-32). Content types are explicit
  (`application/vnd.apple.mpegurl`, `video/mp4`, `video/iso.segment`); the playlist is
  `Cache-Control: no-store`, init and segments `private, max-age=3600`.
- Streaming routes are documented in `api/openapi.yaml` (tag `stream`) but excluded from the
  strict server through oapi-codegen's `output-options.exclude-operation-ids`. They are plain
  `net/http` handlers in `internal/adapters/http/stream`, mounted on the REST API mux, so they
  keep the API's 404/405 problem handling while using Range and long-lived writes.
- No throttling in milestone 2: FFmpeg processes the file to the end. Milestone 4 adds
  throttling and the transcode cache.

**Seeking by restart**
- Seeks inside the range the player already has (direct play: the whole file; HLS: produced
  segments) are local. Any other seek creates a new session at the target position with
  `replacesSessionId`.
- FFmpeg seeks with `-ss` before `-i`. When video is copied, output can only start at a
  keyframe, so the service looks up the last keyframe at or before the target (ffprobe) and
  reports it as the actual start; when video is transcoded the start is exact. Output
  timestamps start at 0. The player shows `position = actual start + currentTime` and uses the
  probed duration for the scrubber.

**Transcode directory**
- Only `<transcode>/sessions/` is managed. At startup the server deletes entries in it whose
  names are session UUIDs, and nothing else, so a misconfigured transcode path cannot destroy
  user files. A single-instance lock on the data directory prevents a second server from
  purging the first one's sessions.

**Process rules** (ADR 0015)
- `exec.CommandContext`, argument slices, no shell, `-nostdin`, `-hide_banner`, inputs and
  outputs prefixed with `file:`. stderr is kept as a bounded tail and logged when a process
  fails.

**Readiness for authentication (milestones 7 and 7b)**
- Clients never build stream URLs; they use the `streamUrl` the server returns, and the HLS
  playlist references its files with relative URIs. Milestone 7 can therefore make URLs signed
  and time-limited (for example a token path segment the server adds) without changing v1
  clients, and relative segment URIs inherit it.
- Sessions get an owner (user, device) in milestone 7; `GET`/`DELETE` then check it. Stream
  routes are `security: []` in the spec until then.
- The access log never logs query strings (M1-09), and stream requests are logged at debug
  level when successful, so segment polling does not flood the log.
- Every API request body is limited to 64 KiB (`http.MaxBytesReader`); the client profile has
  bounded arrays.

## Consequences

- One mechanism (sessions) covers all three modes, the admin dashboard of milestone 4 and the
  ownership checks of milestone 7.
- Without throttling, a long transcode fills the transcode directory up to the size of the
  output; acceptable for the alpha, documented for NAS users.
- A seek outside the produced range costs an FFmpeg restart (about a second for remux, more for
  transcode on weak CPUs).
- The `EVENT` playlist shows only the produced part to the browser's native controls; the web
  player therefore draws its own scrubber over the full probed duration.

## Notes (M2-01)

Clarifications from the API contract (M2-01); they refine the decision above without changing
it.

- `streamUrl` and the `Location` header of a new session are URI references, resolved per
  RFC 3986 section 5 against the URL of the request that returned them. Milestone 2 returns
  root-relative references (`/api/v1/...`), which break behind a reverse proxy that serves the
  server under a path prefix. Supporting such a prefix (configured base path or relative
  references) is a follow-up; decide it before milestone 7b (remote access).

## Notes (M2-20, orchestrator)

- `replacesSessionId` (seeking, track change): the new session takes over the old session's slot
  atomically when it is reserved; the old session keeps running until the new production has
  started and is stopped only then, so seeking never trips a limit and a failed restart leaves
  the old session playable. During a swap the real number of FFmpeg processes can briefly exceed
  `MaxFFmpegSessions` / the transcode limit by the number of replacements in flight.
- Overall cap: at most 256 sessions (`MaxSessions`) including direct play, answered with 503
  `session_limit_reached` – bounds memory while there is no authentication (until M7).

## Notes (M2-25)

Lifecycle details of `app/playback`; they refine the decision above without changing it.

- The idle reaper checks every 15 s (`IdleCheckInterval`) and stops sessions not accessed for
  2 minutes or longer (`IdleTimeout`), so a session ends at most 15 s after its timeout. The
  idle time starts when a session becomes visible (its production has started), and only
  stream requests (`Access`) count, not `GET` of the session.
- A stream request in flight is activity: a browser often streams a direct-play MP4 over one
  long Range request, which would otherwise be reaped mid-playback. `Access` marks the session
  in use until the delivery adapter releases the returned access (`StreamAccess.Release`,
  deferred, idempotent); the reaper skips sessions in use, and the idle time restarts when the
  last request ends. `DELETE`, a replacement and shutdown still end a session in use; the open
  request's reads then fail.
- Direct-play sessions have no cap of their own (this ADR does not require one): they count
  only against `MaxSessions`, and the idle reaper removes forgotten ones.
- An HLS session is `ready` with the first segment, `ended` after a clean FFmpeg exit and
  `failed` with `failureCode` `transcode_failed` after an exit error or a clean exit without
  any segment. The session resource of an ended or failed session
  (`GET /api/v1/playback/sessions/{id}`) stays readable until `DELETE` or the idle timeout, so a
  client can read why it failed. An ended session keeps serving its HLS files; for a failed
  session the delivery adapter answers 500 for every HLS file, including segments written
  before the failure. A stopped session is removed, not ended. The FFmpeg producer logs a failed process once at warn (exit code,
  stderr tail); the playback service records the state change only at info (session, file,
  failure code), so one failure is warned about once.
- When a replacement re-encodes less than the session it replaces (transcode → remux), the old
  session keeps counting in the categories the replacement does not occupy until it is stopped,
  so the limits hold while both run.
- The startup purge runs before the first session only; afterwards it would delete live
  directories and is refused. Sessions requested while it runs get 503 (`ErrUnavailable`).
- Shutdown (`Close`): new sessions get 503 (`ErrServiceClosed`, wraps `ErrUnavailable`), also
  a start that races the shutdown; all sessions are stopped in parallel (Stop first, so a kill
  is not reported as a crash), starts still in flight tear down what they started and a
  running purge finishes, then the process context is cancelled. Close is bounded by its
  context; when it runs out, the cancelled context kills what is left.

## Notes (M2-24)

- Every failed production is logged once at warn by the producer, with the session, the exit
  code, the number of segments written and the tail of FFmpeg's stderr (at most 4 KiB, cut at a
  character boundary). The tail may contain an absolute media path; it stays in the server log
  and is never returned to clients. `app/playback` records the resulting state change at info
  only (Notes M2-25), so a failure produces one warning.
- The producer reads the playlist through the session directory's `os.Root`, and the delivery
  adapter opens playlist and segments the same way and only for the duration of one response.
- When the service context ends without `Stop`, `Ready` reports `ErrProductionStopped` and no
  warning is logged.

## Notes (M2-24b)

Process hardening in `adapters/ffmpeg`; it refines the process rules above.

- **FFmpeg dies with the server**, also when the server crashes or is killed. Linux: every tool
  starts with the parent-death signal `SIGKILL` (`Pdeathsig`). Linux sends it when the OS
  thread that started the child ends, and Go ends a thread when a goroutine locked to it
  returns, so the goroutine that starts a tool stays locked to its thread until it has waited
  for the tool (one OS thread per running tool, which the wait blocks anyway). Locking only
  around the start would not do: afterwards any goroutine could lock that thread and end it.
  The producer therefore starts and waits for FFmpeg on its supervising goroutine, never on
  the caller's. Windows: every tool runs in its own job object with
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`; the server holds the only handle, so Windows ends the
  job's processes when the server exits. A tool joins its job right after the start, so it runs
  for a moment outside it; a server killed in exactly that moment leaves it running. Creating
  the tool suspended (`CREATE_SUSPENDED`) would close that window, but `os/exec` closes the
  handle of the primary thread, and finding the thread again with the documented Toolhelp
  snapshot costs 100–150 ms per start on a desktop with thousands of threads (measured), more
  than a quick ffprobe run takes; the faster ways (`NtResumeProcess`, `NtGetNextThread`) are
  undocumented. Windows refuses (`ACCESS_DENIED`) to assign a tool that is already terminating,
  which a quick ffprobe can be by then (about 1 in 5000 starts under load). Such a tool needs no
  job: its exit code is set, or its handle is signalled within 100 ms. Any other assignment
  failure refuses the start (`ErrUnavailable`) instead of running an unguarded tool. macOS and
  the BSDs have no such guard. Tests kill a stand-in server without cleanup and assert that its
  child is gone (Linux in Docker, Windows locally), and start 1000 instantly exiting tools on
  Windows (100 elsewhere; `MIVO_FFMPEG_STRESS_STARTS` sets the count) without a single
  failure.
- The job limits travel to `SetInformationJobObject` by address through a
  `//go:uintptrescapes` function. `golang.org/x/sys/windows` takes such addresses as plain
  `uintptr` values and hands them on through Go code, so a value on the goroutine's stack can
  move during the call. Before this fix about 6 in a million job objects silently lost
  `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` under GC pressure. A test reads the limits back after
  every bind, 500,000 binds while the collector churns
  (`MIVO_FFMPEG_JOB_STRESS_BINDS` sets the count). Any future call that passes a Go value by
  address to `x/sys/windows` needs the same treatment.
- **Playlist rename on Windows.** FFmpeg's `temp_file` flag replaces `index.m3u8` by renaming
  the temporary file over it. On Windows that rename fails while any reader has the playlist
  open, even with `FILE_SHARE_DELETE` (`os.Root`), and FFmpeg 9.0.2 only logs it and still
  exits with 0. Readers are the producer's readiness poll and the clients' playlist requests.
  A lost rewrite is repaired by the next one, but a lost last rewrite left the playlist without
  its last segments and `#EXT-X-ENDLIST`, plus a stray `index.m3u8.tmp`. After a clean exit the
  producer therefore renames a remaining `index.m3u8.tmp` itself, retrying for about a second
  while readers hold the playlist, and logs a warning if that fails. While FFmpeg runs, a client
  may briefly see the previous playlist version.
- The failure warning carries `exit_code` only when FFmpeg exited: its code, 0 for a clean
  exit without segments, and -1 for a process ended by a signal, as Go reports it. It is left
  out for failures that are no exit, such as a failed wait. `ffmpeg.ErrorOutput` (the stderr
  tail) may be logged; it is never returned to clients.

## Notes (M2-30)

Player decisions of the web app; they refine "Seeking by restart" without changing it.

- **A restart empties the media element right away**; the old stream does not keep playing until
  the new session arrives. A restart means the target lies outside what the element has, so the
  old stream would play the wrong part of the file, move the displayed position away from the
  target the user just picked and keep its audio running, and its requests may fail once the
  server swaps the sessions (Notes M2-20). The player shows the target position on the scrubber
  and a spinner until the new stream plays.
- A seek in a direct-play file is local as soon as the file is attached, also before its first
  frame (a start position that is still pending moves instead). Only an element that has
  already failed restarts.
- Native `controls` stay off: the `EVENT` playlist would show only the produced part. The web
  player draws its own scrubber over the probed duration and shades the produced range of an
  HLS stream (its `seekable` ranges on the file timeline).
- While no stream plays (loading, a restart, a wait for the stream), play and pause set the
  engine's intent (`autoplay` in the snapshot) instead of acting on the empty element, and the
  button shows that intent. The controls hide after 3 s of playback, but never while keyboard
  focus is on one of them.

## Notes (M2-32)

Wiring in `cmd/mivo` and delivery details; they refine the decision above without changing it.

- **HLS delivery**: every file of the HLS route waits for the session's first segment, not only
  the playlist (once it exists the wait returns at once), and a failed session answers 500 for
  every file. The route answers `HEAD` like `GET` and supports Range (`If-Range`, 206, 416) for
  every file, through `http.ServeContent`; `api/openapi.yaml` lists both (`headPlaybackHlsFile`).
- **Startup**: the playback service is built after the FFmpeg capability detection and purges
  `<transcode>/sessions` before the HTTP listener is bound, so the purge can never meet a live
  session. A sessions entry that is no plain directory fails the start and names
  `ErrSessionsNotPlainDirectory`; a leftover that cannot be deleted is only logged (it costs disk
  space, not correctness). A transcode path containing `%` fails the start, because FFmpeg
  would read it as a segment template. The transcode height cap is 1080 (OD-5 B) without a
  configuration key.
- **Missing tools degrade the server instead of stopping it**: `tools.ffmpeg` and
  `tools.ffprobe` are resolved once with `exec.LookPath` (which adds the Windows executable
  extensions also to a configured path) and must be regular files; on Windows a `.bat` or `.cmd`
  file fails the start, because `cmd.exe` re-parses its command line. A tool that does not exist
  is logged at warn and the server starts: without ffmpeg the capabilities stay zero (direct play
  only, `transcode_unavailable` otherwise), without ffprobe scans fail with
  `prober_unavailable`. Such a tool reaches the adapters as an absolute path below
  `<data>/missing-tools/`, which is never created, so every run fails with `ErrUnavailable`
  until a restart resolves the tool again with all checks. The configured bare name would let
  `os/exec` repeat the PATH lookup on every run and start a tool installed later without them.
- **Shutdown fits 18 s** (`shutdownBudget`), below the 20 s after which Docker
  (`--stop-timeout 20`) and the Synology stop script send `SIGKILL`: HTTP server
  (`server.ShutdownTimeout`, 10 s) → playback `Close` (5 s; killed FFmpeg processes normally exit
  within milliseconds; when the bound runs out, the cancelled service context kills what is
  left and the next start purges leftover directories) → scan `Close` (2 s; cancelling kills the
  ffprobe processes) → database → data directory lock (1 s reserved). A test keeps the sum of
  the bounds inside the budget. A second Ctrl+C still ends the process at once, and FFmpeg dies
  with it (Notes M2-24b).
- **Quiet routes**: `server.Options.QuietPatterns` gets exactly the two stream patterns
  (`stream.QuietPatterns()`), so `GET`/`DELETE` of a session stay at info.
- **Path escape**: a catalogued file whose path now leaves its library (`libraryfs` reports
  `domain.ErrForbidden`) is answered 422 `unsupported`, at session start
  (`appplayback.ErrFileOutsideLibrary`, logged at warn by `app/playback`) and on the direct-play
  file route after the session started (logged at warn by the stream handler). 403 `forbidden`
  stays reserved for a missing permission, which since M2-31 also means a client outside the
  LAN, so a player can tell the two apart.
