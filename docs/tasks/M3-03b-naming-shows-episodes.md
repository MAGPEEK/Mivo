# M3-03b Domain: name parser for shows, seasons and episodes (incl. absolute numbering)
Owner: mivo-backend
Depends on: M3-03
Files/areas: `internal/domain/naming/{show.go,season.go,episode.go,absolute.go}` + tests,
`internal/domain/naming/place.go` (only the show-library branch)

## Goal
Files of a show library turn into show → season → episode placements. Shows with absolute
episode numbers (anime) land in one season with the right episode numbers and never appear as
"130 seasons" – the parser defect seen in the Plex walkthrough becomes a permanent regression
test. UX: DT-20 … DT-22 (data), **DT-24**, ADR 0027 §4 "Shows".

## Acceptance criteria
- [ ] `Place(library.KindShows, file)` returns show → season → episode placements, or show →
      extra for extras folders below the show or a season, or `ignored` for samples.
- [ ] Show: the first directory below the root (`Name (2015)` → title `Name`, year 2015, key
      `NormalizeKey(dir)`); a file directly in the root takes the text before its episode
      marker (key `name:<NormalizeKey(title)>`).
- [ ] Season folders `Season 1`, `Season 01`, `Staffel 2`, `S2`, `Specials` (→ 0), `Season 0`;
      other folder names below a show do not set a season.
- [ ] Markers: `S01E02`, `s1e2`, `S2015E01`, `S01E01-E02`, `S01E01E02`, `S01E01-02`,
      `1x02`, `1x02-1x03`, `E02`, `Ep02`, `Episode 2` (season from the folder, else 1). The
      episode title after the marker is kept as the level title (else `Episode N`).
- [ ] **Absolute numbering (DT-24):** a bare number of 1–4 digits after the show title is the
      episode number and the absolute number; season = season folder, else 1. Never split into
      season and episode (`Show 105` → S1E105). Not numbers: resolutions, codec/audio tags,
      CRC32 in brackets, `[Group]` prefixes, `v2` revision suffix (`Show - 05v2` → E5), the
      show year in parentheses.
- [ ] **Regression "Dragon Ball Super":** the 131 names `Dragon Ball Super/Dragon Ball Super -
      001.mkv` … `- 131.mkv` place into exactly one show, one season (1) and 131 distinct
      episodes 1–131; the variants `[Group] Dragon Ball Super - 130 [1080p] [ABCD1234].mkv`,
      `Dragon Ball Super 130.mkv`, `Dragon.Ball.Super.E130.mkv`,
      `Dragon Ball Super - Episode 130.mkv`, `Dragon Ball Super (2015)/Dragon Ball Super -
      130.mkv` each place as season 1, episode 130. A test builds all placements and asserts
      the distinct season keys = 1 and no season number > 1 (named `TestAbsoluteNumbering…`).
- [ ] Files without any marker (incl. air-date names like `Show 2024-01-05`) become episodes
      without a number in the folder's season (or season 1), keyed by their normalised name.
- [ ] At least 50 table rows covering every marker form, multi-episode files, specials,
      season folders with absolute numbers, root-level files, extras folders and samples.
- [ ] Tests: `go test ./internal/domain/naming/...`; coverage ≥ 90 %.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./...`, coverage gate.

## Notes
- Episode key `<episode>` or `<episode>-<end>`; two files with the same key are versions of
  one episode (the version label is the rest of the name after the marker and title only when
  a `- <label>` suffix exists; keep it simple and document it).
- Reuse the tokenizer of M3-03; extend its tag tables there only if a show case needs it, and
  keep the movie tests green.
- Provider episode ordering (absolute/DVD) is milestone 6; do not try to derive seasons from
  absolute numbers.
