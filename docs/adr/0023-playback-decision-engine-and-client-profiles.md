# 0023 Playback decision engine and client capability profiles

- Status: Accepted (2026-10-30, confirmed by project owner)
- Date: 2026-10-30
- Refines: "Grundprinzip jedes Format abspielen" in `docs/plan.md`

## Context

No client plays every format. The server must choose, per file and per client, the cheapest of
Direct Play, Direct Stream (remux) and Transcode, and tell the user why (stream-info overlay).
The choice must be testable without FFmpeg or a browser. Milestone 2 has software encoding only
(hardware acceleration, tone mapping, bandwidth tiers and subtitles arrive in milestone 4).

## Decision

**Where the logic lives**
- `internal/domain/playback` is pure: `Decide(input) (Plan, error)` takes the technical
  description of the file (`domain/media`), the client profile, the server capabilities and the
  request options (selected audio stream, start position). It returns a plan or an error that
  wraps `domain.ErrUnsupported` with a reason code. FFmpeg arguments are **not** domain logic:
  `adapters/ffmpeg` translates a plan into arguments.
- `web/packages/player-core` builds the profile in the browser. The TV app (milestone 10) and
  DLNA renderer profiles (milestone 8) produce the same structure.

**Identifiers** (shared by Go, the API and TypeScript)
- Codecs use ffprobe's `codec_name` (`h264`, `hevc`, `av1`, `vp8`, `vp9`, `mpeg4`,
  `mpeg2video`, `theora`, `aac`, `mp3`, `opus`, `vorbis`, `flac`, `ac3`, `eac3`, `dts`,
  `truehd`, `alac`, `pcm_s16le`, …).
- Containers use Mivo's short names, derived from ffprobe's `format_name` plus the file
  extension (ffprobe reports `matroska,webm` and `mov,mp4,m4a,3gp,3g2,mj2` for whole families):
  `mkv webm mp4 m4v mov 3gp avi mpegts m2ts flv ogg mpeg vob asf mxf rm`. These are the values of
  `expected.container` in the test corpus manifest (ADR 0021).
- Direct-play container keys and the MIME types used both for probing in the browser and as
  `Content-Type` when the file is served:

  | Key | Source containers | MIME type |
  |---|---|---|
  | `mp4` | mp4, m4v | `video/mp4` |
  | `mov` | mov | `video/quicktime` |
  | `webm` | webm | `video/webm` |
  | `mkv` | mkv | `video/x-matroska` |
  | `ogg` | ogg | `video/ogg` |
  | `mpegts` | mpegts, m2ts | `video/mp2t` |
  | `3gp` | 3gp | `video/3gpp` |

  Other containers (avi, flv, mpeg, vob, asf, mxf, rm) are never played directly.

**Client profile** (API schema `ClientProfile`; arrays are bounded, strings are lower-case
identifiers)
- `name`: free text for logs and the overlay, at most 64 characters.
- `directPlay`: list of `{container, videoCodecs[], audioCodecs[]}` the client plays as a
  progressive file.
- `hls`: `{delivery: mse | native | none, videoCodecs[], audioCodecs[]}`: codecs the client
  plays inside HLS with fMP4 segments (hls.js over Media Source Extensions, or native HLS).
- `codecLimits`: list of `{codec, maxBitDepth?, profiles[]?}`; a codec without an entry has no
  extra limit. Example: `{codec: h264, maxBitDepth: 8, profiles: [Constrained Baseline, Main,
  High]}`.
- player-core fills it from `HTMLMediaElement.canPlayType` (progressive),
  `MediaSource.isTypeSupported` (HLS via MSE) and, where available,
  `navigator.mediaCapabilities.decodingInfo` (`supported` must be true). `canPlayType` answers
  `probably` and `maybe` both count as supported. Probe strings use representative RFC 6381
  codec strings (H.264 High 8-bit `avc1.640028`, HEVC Main 10 `hvc1.2.4.L120.B0`, AV1 8/10-bit
  `av01.0.08M.08`/`av01.0.08M.10`, VP9 profile 0/2 `vp09.00.40.08`/`vp09.02.40.10`, AAC LC
  `mp4a.40.2`, MP3 `mp4a.6B`, Opus `opus`, FLAC `flac`, AC-3 `ac-3`, E-AC-3 `ec-3`); the
  table lives in player-core with tests.

**Stream selection**
- Video: the first video stream that is not an attached picture (cover art in MKV/MP4 is a video
  stream with the `attached_pic` disposition), preferring one with the `default` disposition.
- Audio: the requested stream index; otherwise the first stream with the `default` disposition;
  otherwise the first audio stream. A file without audio or without video is allowed.
- Subtitles are ignored in milestone 2 (warning `subtitles_not_supported` when the file has
  any). Milestone 4 adds them.

**Modes, in order of preference** (the plan's three stages)
1. `direct_play`: the source container has a direct-play key, the profile lists that key with the
   selected video and audio codecs, all codec limits hold, and the selected audio stream is the
   one a browser plays by default (browsers cannot reliably switch tracks of a progressive
   file). Delivery: the original file over HTTP with Range.
2. `remux`: all selected streams can be copied, i.e. the profile's `hls` lists their codecs and
   the limits hold. Delivery: HLS with fMP4 segments, every stream copied.
3. `transcode`: at least one stream is re-encoded, even if only audio (the plan places DTS and
   TrueHD under "Transcode"). Each stream is copied when the client supports it inside HLS,
   otherwise re-encoded. The per-stream action is part of the plan and shown in the overlay,
   so an audio-only transcode is visibly cheap.
- Transcode targets in milestone 2: video **H.264 High, 8-bit 4:2:0** (software `libx264`),
  audio **AAC LC stereo** (OD for multichannel deferred to milestone 4). The output height is
  capped at 1080 (OD-5 B, decided 2026-10-30) and only ever scaled down, keeping the aspect ratio.
- If transcoding is needed but the server lacks the encoder, or the client cannot play the
  target inside HLS, `Decide` fails with `transcode_unavailable` or `no_compatible_format`.
  With `hls.delivery = none` only `direct_play` is possible.

**Reasons and warnings** (stable machine-readable codes; clients translate them, ADR 0012)
- Reasons explain why a cheaper mode was not possible, each tagged with the stream it concerns
  (`container`, `video`, `audio`): `container_not_supported`, `video_codec_not_supported`,
  `video_profile_not_supported`, `video_bit_depth_not_supported`, `audio_codec_not_supported`,
  `audio_track_not_default`.
- Warnings describe a degraded result: `hdr_not_tone_mapped` (HDR source transcoded without
  tone mapping until milestone 4), `subtitles_not_supported`, `video_downscaled`.
- Errors: `no_compatible_format`, `transcode_unavailable`.
- New codes may be added within API v1; clients must show a generic text for unknown codes.

**Server capabilities**
- Detected once at startup by `adapters/ffmpeg` (ADR 0015: never assumed): FFmpeg version,
  whether the `hls` and `mp4` muxers and the `libx264` and `aac` encoders exist. Without
  FFmpeg only `direct_play` is possible; without the encoders no `transcode`. The plan's rule
  "without transcoding power only Direct Play and Remux, with a clear note in the UI" maps to
  `transcode_unavailable`.

## Consequences

- The decision is a table-testable pure function (format matrix × reference profiles,
  ADR 0025). Domain coverage ≥ 90 % applies.
- Adding a client type (TV, DLNA) means producing a profile, not changing the engine.
- Browsers whose `canPlayType` underreports (for example Matroska in Chromium) get a remux
  instead of direct play. This is cheap and predictable; per-browser overrides are not added.
