# M3-03 Domain: name parser for movies, versions, parts and extras
Owner: mivo-backend
Depends on: M3-04
Files/areas: `internal/domain/naming/**` (new package: `doc.go`, `tokens.go`, `movie.go`,
`extra.go`, `version.go`, `place.go` + tests)

## Goal
A library-relative path of a movie library turns into a placement: which movie (folder or
loose), title and year, version label and part, or which extra of which movie – or "ignored"
for samples. Pure, table-driven, the base for scan v2. UX: DT-09 (extras), LB-42 data
(versions), ADR 0027 §4 "Movies".

## Acceptance criteria
- [ ] Entry point `Place(kind library.Kind, file FileContext) (Result, error)`;
      `FileContext{Path string (NFC, '/'-separated, library-relative), DirectoryVideoCount
      int}`; `Result{Outcome (placed|ignored), Placement library.Placement}`. Show libraries
      return `ErrUnsupported` until M3-03b; any other kind → `domain.ErrInvalid`.
- [ ] Shared tokenizer/cleaner (`tokens.go`): `.`/`_` as separators unless in a known
      abbreviation, trailing release tags end the title (resolutions `480p`…`4320p`, `4K`,
      `UHD`, codecs `x264 x265 h264 h.264 h265 h.265 hevc avc av1 xvid divx`, `10bit`,
      sources `bluray bdrip brrip web-dl webrip hdtv dvdrip remux`, audio `aac ac3 dts
      truehd atmos ddp5.1 5.1 7.1`, `hdr hdr10 dv`), release groups `-GROUP` and `[Group]`,
      bracketed CRC32 `[ABCD1234]`. Exported for M3-03b.
- [ ] Years: 1880–2100, `(YYYY)`, `[YYYY]` or a bare token followed by release tags; the
      **last** year-like token is the year. Cases: `Blade Runner 2049 (2017)`,
      `2001 A Space Odyssey (1968)`, `1917 (2019)`, `2012.2009.1080p.mkv`,
      `The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv`, a title without year.
- [ ] Folder movie (ADR 0027 §4): base name starts with the folder name, or the only video in
      a non-root directory → title/year from the folder, key `dir:<NormalizeKey(dir)>`,
      version label = remainder without separators and brackets (`Movie (2010) - 4K` → `4K`,
      `{edition-Director's Cut}` → `Director's Cut`). A file in the root is never a folder
      movie. Loose movies: key `<dir>|<title>|<year>` with normalised parts.
- [ ] Parts `cd1 cd2 disc1 disk2 part1 pt2` (separator before, number 1–9) set `PartNumber`
      and do not become part of the version label.
- [ ] Extras: the folder names and suffixes of ADR 0027 §4 (case-insensitive) with the right
      `ExtraType`; a folder extra's parent is the folder movie of the directory above the
      extras folder; a suffix extra's parent is the movie its stripped base name places to;
      a file named `trailer` in a movie folder is a trailer. Placement = movie level + extra
      level (key = parent key + normalised relative path).
- [ ] Samples are `ignored` (whole name `sample`, `-sample`/`.sample` suffix, inside a
      `Sample` folder).
- [ ] At least 40 table rows for movies and 15 for extras/samples, each with the full expected
      placement (kinds, keys, title, year, version label, part).
- [ ] Tests: `go test ./internal/domain/naming/...`; coverage ≥ 90 %.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./...`, coverage gate.

## Notes
- The parser never sees the file system: `DirectoryVideoCount` comes from the scan, which
  counts videos per directory (excluding extras folders and samples).
- Titles keep their original letters (NFC input); only keys are case-folded.
- Leave the show-specific code to M3-03b; `place.go` dispatches by library kind, and M3-03b
  replaces the `ErrUnsupported` branch only.
- Functions ≤ 50 lines, cognitive complexity ≤ 15: prefer small token-classifier helpers and
  data tables over long `switch` chains.
