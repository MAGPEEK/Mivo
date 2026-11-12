# M4-10 FFmpeg: multichannel, hi-res and DSD audio arguments
Owner: mivo-media
Depends on: M4-04c, M4-03
Files/areas: `internal/adapters/ffmpeg/hlsargs.go` (new `audioargs.go`) (+ tests)

## Goal
The audio plan of M4-04c is executed: 5.1 stays 5.1, hi-res PCM is downsampled, DSD plays as PCM in the browser (ADR 0030 §4).

## Acceptance criteria
- [ ] AAC 5.1 target: `-ac 6 -c:a aac -b:a 384k` when the plan keeps six channels, otherwise the M2 stereo downmix with normalised levels.
- [ ] Hi-res: `-af aresample=<rate>:resampler=soxr` only when FFmpeg lists soxr (else the default resampler); `-sample_fmt` per target bit depth. (Detection `HasSoxResampler` exists; passing `HLSJob.SoxResampler` from the composition root is M4-15, criterion amended by the orchestrator 2026-11-12.)
- [ ] DSD (DSF/DFF): `-af aresample=96000` plus a low-pass-friendly `dsd2pcm` path via FFmpeg's decoder (`dsd_lsbf`, `dsd_msbf` …);
      output FLAC (fMP4 FLAC) or AAC per plan; seek by restart works (session start position).
- [ ] Audio-track pick uses `-map 0:<streamIndex>` of the plan; an invalid index fails before FFmpeg starts.
- [ ] Tests: arg golden tests; corpus tests: 5.1 AAC clip stays 6 channels (ffprobe of the first segment), 96 kHz FLAC → 48 kHz, DSF → decodable output.
- [ ] Gates: lint + layer rules + `go test -race ./internal/adapters/ffmpeg/...` + coverage gate.

## Notes
- Music playback uses the same session machinery; no separate path.
- CPU cost of DSD is high on weak NAS; the plan's reason `dsd_to_pcm` (M4-04c) lets the UI warn.
