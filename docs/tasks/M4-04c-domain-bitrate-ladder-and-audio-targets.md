# M4-04c Domain: bitrate ladder, multichannel and hi-res audio targets
Owner: mivo-backend
Depends on: none
Files/areas: `internal/domain/playback/ladder.go`, `transcode.go`, `profile.go` (+ tests)

## Goal
Quality steps (PL-10) and better audio targets are decided in the domain: a bit-rate cap forces a
transcode, 5.1 stays 5.1 for capable clients, DSD and hi-res audio fit the client (ADR 0030 §4, §6).

## Acceptance criteria
- [ ] `Ladder()` returns the fixed steps 20/12/8/4/2/1 Mbit/s with heights 2160/1080/1080/720/480/480; `Request.MaxBitrate`
      0 = original; a value below the source video bit rate forces `transcode` with reason `bitrate_cap`.
- [ ] Target size for a cap = largest ladder height not exceeding source and the cap's height (even dimensions).
- [ ] Audio: profile `maxAudioChannels >= 6` keeps up to 6 channels (AAC 5.1 target) when the codec must be
      transcoded; otherwise stereo downmix (M2 behaviour).
- [ ] DSD (`dsd_*`) is always `transcode` to PCM 24-bit, ≤ 96 kHz; PCM above the profile's `maxSampleRate` /
      `maxBitDepth` is downsampled; lossless-capable profiles get FLAC, others AAC.
- [ ] `ServerCapabilities` gains nothing hardware-related here (that is M4-04d).
- [ ] Tests: table-driven; coverage ≥ 90 %.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
- No magic numbers: ladder is a named table. Cap applies to total stream bit rate; the adapter (M4-11) subtracts audio.
