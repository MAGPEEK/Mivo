# M2-06 Test corpus: seek clips, multiple filters and manifest fixes
Owner: mivo-media
Depends on: none
Files/areas: `scripts/gen-testmedia/**`

## Goal
The corpus gets two 30-second clips with a fixed keyframe interval for seek tests, `-only`
accepts several patterns (the E2E suite builds a small subset), and the manifest stays correct on
partial runs (ADR 0021, ADR 0025).

## Acceptance criteria
- [ ] New cases `video-mkv-h264-aac-long` (H.264 High/AAC in MKV, remux path) and
      `video-ts-mpeg2-mp2-long` (MPEG-2/MP2 in TS, transcode path): 30 s, 25 fps, keyframe every
      2 s (`-g 50 -keyint_min 50 -sc_threshold 0` or the MPEG-2 equivalent); manifest
      `durationSeconds` 30.
- [ ] `-only` accepts a comma-separated list of globs (`-only 'video-mp4-*,video-webm-vp9-opus'`);
      each glob is validated; usage text updated; test.
- [ ] BACKLOG row "testmedia encoders": entries of kept files carry their encoders over from the
      previous manifest (never this run's alternative, never dropped on interrupted or `-only`
      runs); kept files are labelled `kept` in progress and summary instead of `generated`.
      Tests for both.
- [ ] BACKLOG row "testmedia nits": `expected()` sidecar slice aliasing fixed (test that mutating
      one result does not change another); second Ctrl+C via `context.AfterFunc(ctx, stop)`;
      default dispositions set for video/audio in multi-track MKVs.
- [ ] Real run: `go run ./scripts/gen-testmedia -force` → 45 generated, 9 known gaps, 0 failed;
      a rerun without `-force` keeps all files byte-identical.
- [ ] `ffprobe -skip_frame nokey -select_streams v:0 -show_entries frame=pts_time -of csv` on the
      long MKV lists keyframes 2 s apart (quote in REPORT).
- [ ] Gates: `go vet ./scripts/...`, `golangci-lint run ./scripts/...`, `go test -race ./scripts/...`
      with coverage ≥ 90 % for the package.

## Notes
- Add a `withDuration` source option rather than a second constant; the clip length stays 2 s
  for all other cases.
- Keep output deterministic (bitexact flags as today).
- Do not change the manifest schema version unless consumers must adapt; adding cases does not
  require it.
