# M3-04 Domain: catalog model v2 (item kinds, placement, file classes, sort titles)
Owner: mivo-backend
Depends on: M3-01
Files/areas: `internal/domain/library/**` (new files `placement.go`, `extra.go`, `key.go`,
`file_class.go` + tests; additive changes to `item.go`, `file_state.go`, `file_policy.go`,
`title.go`), `docs/tasks/BACKLOG.md` (only the two `domain/library` rows)

## Goal
The domain knows shows, seasons, episodes, movies and extras, can describe where a file belongs
in that hierarchy (a placement with stable match keys) and classifies library files into
videos and sidecars. Every later M3 task builds on these pure types. UX: DT-20 … DT-22 (data),
LB-22 (sort titles), DT-09 (extra types).

## Acceptance criteria
- [ ] `ItemKind` adds `show`, `season`, `extra`; `ParseItemKind` accepts all five video kinds;
      `ItemKind.OwnsFiles()` (movie, episode, extra) and `ValidParent(child, parent ItemKind)`
      implement the table of ADR 0027 §2 (movie/show: no parent; season → show; episode →
      season; extra → movie or show).
- [ ] `ExtraType` with the eight values of ADR 0027 §4 and `ParseExtraType`.
- [ ] `FileClass` `video|nfo|image` and `ClassifyFile(name string) (FileClass, bool)`: the
      existing video extensions, `.nfo`, and `jpg jpeg png webp tbn` (case-insensitive);
      everything else is not a library file. The existing video policy keeps its behaviour.
- [ ] `FileState` gains `Class FileClass` and `NormalizedPath string` (NFC form provided by the
      adapter; empty means "same as RelativePath"); `FileState.PathForParsing()` returns it.
      Existing change detection (`Changed`) is unchanged.
- [ ] `Placement` = ordered levels (root first, leaf last) of `{Kind, MatchKey, Title, Year,
      Number, NumberEnd, AbsoluteNumber, ExtraType}` plus `VersionLabel` and `PartNumber` of the
      file. Numbers distinguish "absent" from 0 (season 0 = specials). `Placement.Validate()`
      rejects: no level, a non-file-owning leaf, a parent that `ValidParent` forbids, an empty
      key or title, negative numbers, `NumberEnd` < `Number`, `ExtraType` on a non-extra.
      `Placement.Key()` returns a canonical text of the chain (kinds and keys), version label
      and part, so stored and computed placements can be compared.
- [ ] `NormalizeKey(text)` folds case (`unicode`), maps every run of non-letters/non-digits to
      one space and trims; documented as expecting NFC input (ADR 0027 §3).
- [ ] `Item` gains `ParentID`, `MatchKey`, `Year`, `Number`, `NumberEnd`, `AbsoluteNumber`,
      `ExtraType` (additive; existing fields and constructors keep working).
- [ ] `SortTitle` drops one leading article `The`, `A`, `An`, `Der`, `Die`, `Das`, `Ein`,
      `Eine` (whole word, case-insensitive, only when a title remains) before case folding.
- [ ] BACKLOG `domain/library` nits row done (regression rows for `//server`, `\\?\`, `\\.\`,
      `%2e%2e`, `...`, full-width dots; `ItemKind.IsValid`; `MaxNameLength` in tests;
      whitespace-only root; `nulCharacter` placement; the unreachable "Untitled" fallback
      decided and documented) and the NFC row closed by the boundary rule (ADR 0027 §3:
      adapters normalise, the domain expects NFC). Delete both rows.
- [ ] Tests: table-driven, no mocks, `go test ./internal/domain/...`; coverage of
      `internal/domain/library` ≥ 90 %.
- [ ] Gates: `go vet ./...`, `golangci-lint run` (incl. depguard: standard library only),
      `golangci-lint fmt --diff`, `go test -race ./...`, coverage gate.

## Notes
- Keep `ItemKindForLibrary` and the M2 behaviour of `TitleFromFileName` untouched: the M2 scan
  and sqlite writer still use them until M3-10/M3-05b; M3-10 removes the then unused helper.
- Changes must be additive so M3-05 (sqlite migration, same wave) keeps compiling against the
  old and the new package.
- Do not import `golang.org/x/text`: the domain is standard-library only (ADR 0019).
- `metadata` and `artwork` types are M3-04b (separate packages, same wave).
