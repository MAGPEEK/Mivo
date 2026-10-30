# M1-19 gen-testmedia: audio, image, subtitle and multi-track cases
Owner: mivo-media
Depends on: M1-18
Files/areas: `scripts/gen-testmedia/cases_audio.go`, `cases_image.go`, `cases_subtitle.go`,
one registry line in `cases.go`, sidecar text writer, tests

## Goal
The corpus also covers audio formats, still images, subtitles and multi-track files. Known
gaps are listed in the manifest instead of being silently missing.

## Acceptance criteria
- [ ] Audio cases (5 s `sine`, stereo): FLAC 16/44.1, FLAC 24/96, ALAC (`.m4a`), MP3, AAC
      (`.m4a`), Opus (`.opus`), Vorbis (`.ogg`), WAV PCM, AIFF, WavPack (`.wv`), M4B with two
      chapters (via an FFmetadata file the program writes) and title/artist/album tags.
- [ ] Image cases: JPEG, PNG, WebP (lossy), animated GIF, TIFF, AVIF (if an AV1 still
      encoder exists).
- [ ] Subtitle cases:
      - sidecar `.srt`, `.vtt` and `.ass` next to a video file, written by the program as
        text (the VTT and ASS files either written directly or converted from the SRT by
        FFmpeg)
      - MKV with embedded SRT and ASS tracks
      - MP4 with `mov_text`
- [ ] Multi-track case: MKV with two audio tracks (`eng`, `deu` language tags, one default)
      and two subtitle tracks (`eng`, `deu`, one forced).
- [ ] Gap entries in the manifest with status `skipped` and reason `no encoder in ffmpeg`
      (or similar): APE, DSF, DFF, HEIC, PGS, VobSub, DVB subtitles, EXIF-tagged JPEG
      (ADR 0021).
- [ ] Tests (no FFmpeg): argument building for the M4B chapter case and the multi-track case
      (correct `-map`, `-metadata:s:a:0 language=eng`, `-disposition` values); sidecar
      writer output (valid SRT timing format); gap entries present in the manifest. An
      integration test (skips without FFmpeg) generates the multi-track case and asserts
      stream count and language tags through `ffprobe -print_format json`.
- [ ] Running the full program on the dev machine succeeds. The REPORT shows generated,
      skipped and gap counts per category.
- [ ] Gates: `golangci-lint run`, `go test ./...` and the coverage check pass.

## Notes
- FFmpeg cannot convert text subtitles to bitmap subtitles, so PGS/VobSub/DVB need sample
  files later (separate task, licensing to be checked).
- Keep each case-declaring function under 50 lines. Split by sub-category if needed.
- Use language tags in ISO 639-2 (`eng`, `deu`) as players and Matroska expect.

## Review carry-over from M1-18 (APPROVE)
- Add "RealVideo 4 (RMVB)" to the manifest gap entries (ADR 0021); the `.rmvb` case stays an RV20 placeholder.
- `manifest.go`: chmod the temp manifest to a named 0o640 before rename (readable by group, matching 0750 dirs).
- Interrupted runs (Ctrl+C / SIGTERM): return a wrapped "interrupted" error, do not mark untouched cases `failed`; handle `syscall.SIGTERM` in `signal.NotifyContext`.
- Nits: no trailing ": " when stderr is empty; keep the stderr tail on a UTF-8 rune boundary; HDR10 expectation also checks colour matrix `bt2020nc`; summary counts "not selected" separately from capability skips; optionally record the encoder used (schema version bump); remove redundant `gen.jobs = 0` in runner_test.go.
