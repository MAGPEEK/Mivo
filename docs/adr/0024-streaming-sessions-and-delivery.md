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
  then answers 503 `stream_not_ready` with `Retry-After`. Content types are explicit
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
