# 0030 Streaming extension (milestone 4): subtitles, tracks, hardware, throttling, trickplay

- Status: Accepted (orchestrator decision 2026-11-04, owner delegated; open decisions OD-M4-1..4 follow the recommendations in M4-00) (2026-11-04, mivo-architect; binding for implementers until the owner decides)
- Date: 2026-11-04
- Refines: ADR 0023 (decision engine), ADR 0024 (sessions and delivery); supersedes nothing

## Context

Milestone 2 ships direct play, remux and software transcode (libx264, AAC stereo) with
sessions, an idle reaper and limits. Subtitles are ignored, the audio track is the only
selectable stream, nothing is throttled, there is no hardware encoding and no seek preview.
The reference NAS (Synology DS925+, AMD Ryzen R1600-class CPU, **no iGPU**) can only encode
in software, so every default must be safe on 2 cores / 4 threads while the server keeps
serving the web app and scanning.

## Decision

**1. Software is the baseline, hardware is an optimisation.** Nothing in M4 may make the
software path slower or less safe. Software defaults: `libx264 -preset veryfast`
(`transcode.preset`, allowed `ultrafast`..`medium`), `transcode.threads` 0 = auto =
`max(1, NumCPU/2)`, FFmpeg runs at low OS priority (Linux nice 10, Windows
BELOW_NORMAL_PRIORITY_CLASS), `transcode.max_sessions` default stays the existing
`DefaultTranscodeMaxSessions`, but M4-05a lowers it to 1 when `NumCPU <= 4`.

**2. Throttling by pausing the process (Linux), never by killing.** FFmpeg's HLS muxer has
no pacing. When a session's newest segment is more than `transcode.throttle_ahead_seconds`
(default 60, expert key) ahead of the highest segment the client has requested, the service
sends `SIGSTOP`; it sends `SIGCONT` when the lead falls below half of that. Paused FFmpeg
consumes no CPU and keeps its position. Windows and macOS have no throttling (dev machines;
documented, `Throttler` reports unsupported). A paused session still counts as running FFmpeg
for limits, and a paused process is resumed before `Stop`/kill. The idle reaper's timing is
unchanged. This also bounds transcode-directory growth to about the lead plus the played part.

**3. Subtitles: sidecar first, burn-in only when unavoidable.**
- Text formats (SubRip, ASS/SSA, WebVTT, mov_text, text-like embedded and external files) are
  delivered as **WebVTT** from `GET /api/v1/media-files/{fileId}/subtitles/{trackId}.vtt`, keyed
  by file and track, not by session. The player adds the session's actual start offset
  (ADR 0024 seeking) to cue times. Conversion by FFmpeg, result cached under
  `<cache>/subtitles/`, bounded in size. No session restart when the user switches text
  subtitles.
- ASS/SSA styling is dropped in 0.1.0 (plain cues). Styled rendering in the browser (JASSUB,
  libass WASM) is a post-0.1.0 task (M4-19); until then ASS styling needs burn-in.
- Image formats (PGS, VobSub, DVB) and any track the client cannot render are **burned in**.
  Burn-in forces a video transcode (decision reason `subtitle_burn_in`), counts against
  `transcode.max_sessions` and is refused with warning `subtitle_burn_in_unavailable` (track
  stays off) when software transcoding is disabled or the session limit is reached. The
  overlay names the reason.
- External subtitle files (`Movie.en.srt`, `Movie.de.forced.ass`, `Subs/` folders) are found
  by the scan next to a media file, stored with language and flags parsed from the name, and
  appear as tracks with `external: true`. Reading is confined to library roots like media.
- Subtitle selection default: client profile language list; forced tracks follow the audio
  language; "off" is a valid default. Choice is remembered per item on the device (AD-02);
  server-side per profile arrives with M7.

**4. Audio.** Track choice stays `audioStreamIndex` on session creation (ADR 0024), changing a
track replaces the session. New: a pure domain default policy (language list, default flag,
no commentary, most channels), multichannel output when the profile says `maxAudioChannels >= 6`
(AAC 5.1; passthrough where the profile lists the codec), DSD (DSF/DFF) always decoded to PCM
(24 bit, 88.2/96 kHz max, FLAC for lossless clients) and hi-res PCM downsampled to the
profile limit.

**5. Hardware acceleration.** `transcode.hwaccel`: `auto` (default), `off`, `vaapi`, `qsv`,
`nvenc`, `amf`; `transcode.hwaccel_device` for VAAPI (`/dev/dri/renderD128`). Detection runs
at startup, in the background, and on request ("Test hardware acceleration", AD-25): for
every API the FFmpeg build lists, the adapter **encodes one second of a generated test source**
with the matching H.264 (and HEVC where listed) encoder on the candidate device, with a 10 s
timeout. Only encoders that complete are reported usable. `auto` picks the first usable in the
order NVENC, QSV, VAAPI, AMF; nothing usable means software, silently, with one info log. If a
hardware session fails before its first segment, the service retries once in software and marks
that encoder `degraded` for the rest of the process lifetime. On the DS925+ the result is
"none", which is correct. Decode stays software unless the encoder API is paired
with its decoder in the tested combination (hardware decode is part of the test, not assumed).
HDR sources are not tone-mapped in 0.1.0 (ADR 0023 warning `hdr_not_tone_mapped` stays).

**6. Bandwidth tiers.** The request carries `maxBitrate` (bit/s, 0 = original). A fixed ladder
(20, 12, 8, 4, 2, 1 Mbit/s with 1080p/720p/480p heights) lives in the domain; a choice below
the source forces a transcode capped with `-maxrate`/`-bufsize`. "Automatic" is a client
decision in player-core from measured segment throughput; the server only honours the
number it is given.

**7. Session management.** Sessions expose client, item, mode, encoder, output bit rate and
transcode speed (parsed from FFmpeg `-progress pipe:1`, throttled to one update per second).
`GET /api/v1/playback/sessions` lists them (management guard of ADR 0028 until M7); stopping
uses the existing `DELETE`. No persistence.

**8. Trickplay.** On demand, never during a scan: when a user opens an item whose trickplay
is missing, a background job at low priority creates one JPEG sprite sheet set (160 px wide
tiles, interval 10 s, 10x10 tiles per sheet) with a single FFmpeg keyframe-only pass (`-skip_frame
nokey`), stored under `<cache>/trickplay/<fileId>/` with a JSON manifest. Generation runs only
when no transcode is running (at most one job at a time) and is skipped when `transcode.max_sessions`
is 1 and a session is active. Missing trickplay is never an error for the player.

**Deferred beyond 0.1.0 (rationale in `docs/tasks/M4-00-overview.md`).** HDR to SDR tone
mapping, media segments and intro detection, chapter images and panel, styled ASS, generated
episode stills.

## Consequences

- A weak NAS never runs more than one re-encode by default, at low priority; throttling keeps
  disk and CPU use bounded on Linux/Docker/SPK.
- Text subtitles cost nothing on the server and do not interrupt playback; image subtitles are
  the only subtitle path that can need a transcode.
- Hardware support is verified by an actual encode, so a Docker container without
  `/dev/dri` or a NAS without iGPU degrades to software instead of failing sessions.
- Windows has no throttling; documented as a dev-machine limitation.
- Sidecar WebVTT keyed by file means cue timing is relative to the file; the player owns the
  offset, a small amount of logic in `player-core` (tested there).
