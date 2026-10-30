# M2-02 Domain: technical description of media files
Owner: mivo-media
Depends on: none
Files/areas: `internal/domain/media/**` (new package)

## Goal
A pure model of what ffprobe tells us about a file – container, duration, bit rate and streams
with codec, HDR and track flags – that the scanner stores, the decision engine reads and the API
shows as technical chips (ADR 0022, ADR 0023).

## Acceptance criteria
- [ ] Types: `Container` (Mivo short names of ADR 0023), `StreamKind` (video, audio, subtitle,
      other), `Stream` (index, kind, codec, profile, level, pixel format, bit depth, width,
      height, frame rate, colour primaries/transfer/space, HDR, channels, channel layout, sample
      rate, language, title, default, forced, attached picture, bit rate), `Description`
      (container, format name, duration, bit rate, streams) and `File` (ID, item ID, library ID,
      relative path, size, modification time, description, probe error code).
- [ ] `IdentifyContainer(formatName, extension)` maps ffprobe `format_name` plus extension to a
      `Container`; table test covers every container of the corpus manifest (mkv, webm, mp4,
      m4v, mov, 3gp, avi, mpegts, m2ts incl. `.mts`, flv, ogg, mpeg, vob, asf, mxf, rm incl.
      `.rmvb`) and unknown input → `ContainerUnknown`.
- [ ] `DirectPlayKey(container)` and `MIMEType(key)` implement the ADR 0023 table (mp4/m4v →
      `mp4`, mpegts/m2ts → `mpegts`, …; avi, flv, mpeg, vob, asf, mxf, rm → none).
- [ ] `ClassifyHDR(stream)` returns `hdr10` (BT.2020 + `smpte2084`), `hlg` (`arib-std-b67`) or
      none; Dolby Vision detection is out of scope (milestone 4) and documented as such.
- [ ] `BitDepth` falls back to the pixel format (`yuv420p10le` → 10) when ffprobe reports no
      `bits_per_raw_sample`.
- [ ] Stream selection helpers: `PrimaryVideo()` skips attached pictures and prefers `default`;
      `DefaultAudio()` prefers `default`, else the first; `AudioByIndex(i)` returns
      `domain.ErrNotFound` for a missing or non-audio index.
- [ ] Validation: `Description.Validate()` rejects negative durations/sizes and duplicate stream
      indexes with `domain.ErrInvalid`.
- [ ] Tests: table-driven, no mocks; `go test -cover ./internal/domain/media/` ≥ 90 %.
- [ ] Gates: lint + layer rules (`internal/archtest`, depguard) + coverage gate pass.

## Notes
- No ffprobe JSON parsing here (that is `adapters/ffmpeg`, M2-11); this package only knows
  normalised values. Codec names are ffprobe `codec_name` strings.
- Library, item and file-name policies live in `internal/domain/library` (M2-03, parallel).
  This package must not import it; `File.RelativePath` is a plain slash-separated string that
  `library.ParseRelativePath` validated at the boundary.
- Durations as `time.Duration`; frame rate as a rational or float with a documented rounding.
- Package doc comment explains the split media (technical) vs library (organisation).
