# M2-17 FFmpeg adapter: HLS remux and transcode arguments
Owner: mivo-media
Depends on: M2-09, M2-11 (package and process runner)
Files/areas: `internal/adapters/ffmpeg/hlsargs.go` (+ tests)

## Goal
A playback plan becomes a complete, safe FFmpeg argument list that writes fMP4 HLS into a
session directory, copying or encoding each stream as planned and starting at a seek position
(ADR 0024).

## Acceptance criteria
- [ ] `HLSArguments(job) ([]string, error)` with `job = {plan, input path, start position,
      output dir}`; returns `domain.ErrInvalid` for a `direct_play` plan or empty paths.
- [ ] Always present: `-hide_banner -nostdin -loglevel error -y`, `-ss <start>` before `-i` only
      when start > 0, `-i file:<input>`, explicit `-map 0:<video index>` / `-map 0:<audio index>`,
      `-sn -dn -map_metadata -1 -map_chapters -1`, `-f hls -hls_time 6 -hls_playlist_type event
      -hls_segment_type fmp4 -hls_fmp4_init_filename init.mp4 -hls_flags
      independent_segments+temp_file -start_number 0 -hls_segment_filename
      file:<dir>/seg-%05d.m4s file:<dir>/index.m3u8`.
- [ ] Copy: `-c:v copy` (plus `-tag:v hvc1` for HEVC), `-c:a copy`.
- [ ] Transcode video: `libx264 -preset veryfast -crf 23 -profile:v high -pix_fmt yuv420p`,
      keyframes forced at segment boundaries (`-force_key_frames expr:gte(t,n_forced*6)`),
      `-sc_threshold 0`, and `-vf scale=-2:<height>` only when the plan downscales. Transcode
      audio: `aac -ac 2 -b:a 192k`. Values are named constants.
- [ ] Table tests: one row per plan shape (remux, video-only transcode, audio-only transcode,
      both, downscale, seek start, no audio, HEVC copy).
- [ ] Integration test (skip without FFmpeg/corpus, `-short`): run the arguments for three corpus
      files (remux MKV H.264, transcode AVI, audio-only transcode M2TS) into `t.TempDir()`; the
      playlist ends with `#EXT-X-ENDLIST`, `init.mp4` + segments concatenated probe as the planned
      codecs.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Check every option on FFmpeg 7.1 (Docker image) as well as 9.0.2: run the integration test
  once inside a container (`docker run` with the image's ffmpeg or a `golang` image with
  `apt-get install ffmpeg`) and note the result in the REPORT. Older FFmpeg may need
  `-strict experimental` for FLAC/Opus in MP4.
- MPEG-TS input with AAC (ADTS) needs no manual `aac_adtstoasc` for the mp4-based muxer; verify.
- With `-ss` before `-i` the output timeline starts at 0 (ADR 0024); do not add `-copyts`.
