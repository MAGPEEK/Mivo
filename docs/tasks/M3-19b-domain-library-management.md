# M3-19b Domain: library v2 definition, folder rules, browse name policy
Owner: mivo-backend
Depends on: M3-04, ADR 0028 written (M3-19 Notes)
Files/areas: `internal/domain/library/` new files `library_v2.go`, `folders.go`,
`options.go`, `language.go`, `browse_policy.go` and their `_test.go`; `definition.go` only to
reuse `validateName`/`validateRoot` (no behaviour change)

## Goal
The pure rules of library management live in the domain, so the use cases, the folder browser
and the configuration import judge a library the same way: what a valid library is, which
folders may belong to it, which folder names the browser hides, and what a valid metadata
language is. UX: LS-01, LS-02, LS-03 (M3 subset).

## Acceptance criteria
- [ ] Types: `Folder {ID, Path}`, `Options {ScanExtras, ReadNFO, LocalArtwork bool}` with
      `DefaultOptions()` (all true), `Library` v2 (`ID, Name, Kind, Folders, MetadataLanguage,
      Options, ImportedFromConfig`), constants `MaxFolders = 16`.
- [ ] `ValidateLibrary(candidate, rules FolderRules) error` checks name (existing rules), kind
      against an allowed set passed in (the second wave enables kinds per task), 1 …
      `MaxFolders` folders, language, and the folder rules below; errors wrap
      `domain.ErrInvalid` and carry a typed reason and the folder index
      (`FolderError {Index, Reason, OtherLibraryID}`; reasons `duplicate`, `nested`, `overlap`,
      `reserved`; `KindError`, `NameError`, `LanguageError`, `TooManyFoldersError` or one
      error type with a reason enum – the implementer chooses, the HTTP adapter must be able to
      map each reason to the problem codes of M3-19a).
- [ ] Folder rules on cleaned absolute paths (`FolderRules {CaseInsensitive bool, Reserved
      []string, OtherLibraries []Library}`; separators are normalised by the caller): within the
      library unique and not nested; not equal to, inside or containing a folder of another
      library (`overlap`, names that library); not equal to, inside or containing a reserved
      directory (config, data, cache, transcode). Prefix tests are per path element
      (`/media/movie` does not contain `/media/movies2`); `CaseInsensitive` folds case (Windows).
- [ ] `FolderKey(path, caseInsensitive) string` – the comparison key the store uses for its
      uniqueness index.
- [ ] `ParseMetadataLanguage(text)`: empty (server default) or `ll`/`lll` with an optional
      `-RR` region (lowercase language, uppercase region, normalised from any case); anything
      else `ErrInvalid`.
- [ ] `IsHiddenFolderName(name) bool` (browse policy): leading `.`; leading `@` (DSM system
      folders such as `@eaDir`, `@appdata`, `@appstore`, `@tmp`); `#recycle`, `#snapshot`,
      `$RECYCLE.BIN`, `System Volume Information`, `lost+found` (ADR 0022 list, case-insensitive
      where ADR 0022 is). Windows attributes are checked by the adapter, not here.
- [ ] Tests: table-driven, no mocks, for every rule and reason, Windows-style case folding,
      element-wise prefixes, language normalisation, hidden names (including names that only
      look similar, e.g. `recycle`, `a@b`); domain coverage ≥ 90 %.
- [ ] Gates: `go vet ./...`, `golangci-lint run` (depguard: standard library only),
      `golangci-lint fmt --diff`, `go test -race ./internal/domain/...`, coverage gate.

## Notes
- No I/O and no `path/filepath` (its behaviour depends on the build OS); compare with
  `strings` on paths the caller has cleaned and given `/` or `\` consistently – take the
  separator as part of `FolderRules` if needed.
- DSM forbids `@` and `#` in share names (DSM-NOTES "Shared-folder names"), so hiding `@…` and
  `#…` at volume level never hides a share.
- `Definition` (milestone-2 config definition) stays for the import seed; add a helper that
  turns a `Definition` into a one-folder `Library` candidate.
- The second wave (M3-13 … M3-15) adds kinds to the allowed set passed in by `cmd`; the domain
  constant list of kinds is extended by those tasks, not here.
