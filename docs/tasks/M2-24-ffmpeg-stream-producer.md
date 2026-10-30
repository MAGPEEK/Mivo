# M2-24 FFmpeg adapter: HLS stream producer
Owner: mivo-media
Depends on: M2-11, M2-17, M2-20 (producer port)
Files/areas: `internal/adapters/ffmpeg/producer.go` (+ tests)

## Goal
A session's remux or transcode runs as a supervised FFmpeg process: it reports when the first
segment is ready, can be stopped at any time, and surfaces failures with a useful log line
(ADR 0015, ADR 0024).

## Acceptance criteria
- [ ] `Producer.Start(ctx, job)` builds arguments with `HLSArguments`, starts FFmpeg through the
      runner and returns a production that satisfies the `app/playback` port: `Ready(ctx)`,
      `Stop()`, `Done()` (plus the exit error after `Done`).
- [ ] `Ready` returns once `index.m3u8` exists and lists at least one segment (poll with a named
      interval; `temp_file` guarantees complete files), returns the process error if FFmpeg exits
      before that, and `ctx.Err()` when the caller gives up. Multiple concurrent `Ready` callers
      are fine.
- [ ] `Stop` kills the process, waits for it (bounded by a named timeout), is idempotent and safe
      to call concurrently with `Ready`.
- [ ] A non-zero exit after readiness is logged once at warn level with session ID, exit code and
      the bounded stderr tail; a kill by `Stop` is not logged as an error.
- [ ] Integration tests (skip without FFmpeg/corpus, `-short`): remux the MKV H.264 clip until
      `Done` with `#EXT-X-ENDLIST`; transcode the long TS clip and `Stop` after `Ready` → process
      gone (no FFmpeg child left; check via `Done`), directory removable on Windows right after
      `Stop`; a corrupt input file → `Ready` returns an error containing the exit code.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- The process context comes from the playback service (`Start`'s ctx), not from an HTTP request.
- No `fsnotify` dependency; polling every 100–200 ms is enough for 6 s segments.
- On Windows `exec.CommandContext` uses `TerminateProcess`; FFmpeg may leave a `.tmp` file, which
  the workspace removal handles.

## Orchestrator note (from M2-11 review)
- Every ffmpeg invocation passes `-protocol_whitelist file` before its input (see ADR 0022
  "Notes (M2-10 review, orchestrator)", path safety); assert it in the argument tests. Consider
  packet keyframe flags (`-show_entries packet=pts_time,flags`) instead of decoded frames for seek
  points.
- (from M2-12 review) On Linux/Docker close the Resolve→FFmpeg TOCTOU: open the media file
  through `libraryfs` (os.Root), pass it via `cmd.ExtraFiles` and give FFmpeg
  `file:/proc/self/fd/3` (seekable, exactly that inode). Keep `Resolve` on Windows.
- (from M2-20) Implement the app/playback `Production` port: `Ready(ctx) error`, `Stop()`,
  `Done() <-chan struct{}`; after Done is closed, Ready returns immediately with the exit result
  (nil = clean end, error = failed). M2-25 and M2-29 rely on this.
- (from M2-20 r2) After `Stop`, `Production.Ready` returns `ErrProductionStopped` (wraps
  domain.ErrNotFound) – answer 404/410-like, never 500; a clean exit without any segment returns
  an error.
- (from M2-17 review) StartPosition for copied video must be passed UNCHANGED from
  `KeyframeAtOrBefore` (with `-copypriorss 0` a start even slightly after the keyframe drops it and
  the video starts a GOP later); HLSArguments already adds -copypriorss 0 and aac_adtstoasc.
