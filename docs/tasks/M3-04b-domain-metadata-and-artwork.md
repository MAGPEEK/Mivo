# M3-04b Domain: metadata record, merge with field locks, artwork rules
Owner: mivo-backend
Depends on: M3-01
Files/areas: `internal/domain/metadata/**` (new), `internal/domain/artwork/**` (new)

## Goal
Pure rules for what an item says about itself and which picture it shows: a metadata record,
the merge of ordered sources that never overwrites a locked field, provider IDs, and the
artwork rules (local file names, selection, allowed widths, cache tags). Milestone 6 plugs
TMDB and the edit dialog into exactly these rules. UX: ED-02 (locks), ED-05 (artwork
selection), ED-07 (provider IDs), LB-44 (widths), DT-06/DT-08 (record fields).

## Acceptance criteria
- [ ] `metadata.Record` with the fields of ADR 0027 §6; `Credit{Name, Role, Character,
      Order}` with `Role` `actor|director|writer|producer|guest_star`; `Date` value type
      (`ParseDate("2024-01-05")`, rejects impossible dates, `String()` round-trips).
- [ ] `metadata.Provider` `tmdb|imdb|tvdb` and `ValidateProviderID` (`tmdb`/`tvdb` digits
      without leading zero, `imdb` `tt` + 7–10 digits).
- [ ] `metadata.Field` lists the lockable fields of ADR 0027 §6 with their stored text values;
      `ParseField`; `LockSet` (from field list, `Has`).
- [ ] `metadata.Source` `filename|nfo` now, with reserved values `embedded`, `tmdb`,
      `musicbrainz`, `user` documented as later; `Merge(current Record, locks LockSet,
      sources []SourcedRecord) Record`: scalar fields from the first source with a value, list
      fields whole from the first source with any value, locked fields keep `current`. Table
      tests: lock on every field, empty sources, list from the second source, `user` source
      first.
- [ ] `metadata.SortTitleFor(record)`: `<sorttitle>` if present, else `library.SortTitle(title)`.
- [ ] `artwork.Kind` `poster|backdrop|thumb|logo|banner`, `artwork.SourceKind`
      `local|embedded|provider|upload`, `artwork.Ref{ID, Tag}`.
- [ ] `artwork.ClassifyLocal(context LocalContext, fileName string) (Kind, priority int, ok)`
      for the name table of ADR 0027 §8; `LocalContext` says whether the folder belongs to a
      folder movie, a loose movie (with its base name), a show folder, a season folder (with
      its number) or an episode (with its base name). `seasonNN-poster` names return the
      season number.
- [ ] `artwork.Select(candidates []Candidate, locked bool, current *Candidate)`: locked keeps
      `current`; otherwise local by priority, then embedded; deterministic tie-break by
      relative path.
- [ ] `artwork.Width` with the set `160 320 480 640 960 1280 1920`, `ParseWidth` (anything
      else → `domain.ErrInvalid`), `artwork.OutputFormat(kind)` (`png` for `logo`, else
      `jpeg`), and `artwork.Tag(identity TagInput) string` = first 16 hex digits of SHA-256
      over source kind, location, size, modification time (ms) and a `RenderVersion`
      constant.
- [ ] `artwork.Stored` (location of one artwork: library root + relative path, or library
      root + media file relative path + stream index, or data-directory path; plus size,
      modification time and tag) and `artwork.RenderRequest{AbsolutePath, StreamIndex
      (-1 = whole file), MaxWidth, Format}` / `artwork.Rendered{Data, Width, Height,
      Format}` – shared by M3-08 (renderer), M3-09 (images) and M3-05c (reader) so they can
      run in parallel.
- [ ] Tests: table-driven, no mocks, `go test ./internal/domain/...`; coverage of both
      packages ≥ 90 %.
- [ ] Gates: `go vet ./...`, `golangci-lint run`, `golangci-lint fmt --diff`,
      `go test -race ./...`, coverage gate.

## Notes
- `crypto/sha256` and `encoding/hex` are standard library and allowed in the domain.
- `metadata` may import `domain/library` (for `SortTitle`) but not the other way round.
- No I/O, no clock: dates and times are passed in.
- Keep `Merge` within the complexity limits: one small helper per field group is fine.
