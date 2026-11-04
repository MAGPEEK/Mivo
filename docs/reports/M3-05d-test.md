# M3-05d test report

Status: PASS (round 1 of max 3)

## Gates
- go vet ./...: clean
- go test -race -coverprofile ./...: all packages ok; sqlite 90.5 %
- go-test-coverage: total 95.1 % (>= 70 %), domain packages 100 % (>= 90 %)
- golangci-lint run: 0 issues
- golangci-lint fmt --diff: no diff
- go mod tidy -diff: clean
- sqlc (Docker 1.31.1) generate: tracked generated files unchanged; the new generated files
  (untracked) were rewritten and compile/pass. Drift for those cannot be shown by git diff
  until they are staged.
- Web/E2E/Docker build: not applicable.

## Acceptance criteria
- Sub-interface assertion `var _ scan.MetadataCatalog = (*LibraryCatalog)(nil)`
  (library_catalog_metadata.go:21): met. CatalogV2 assertion only after M3-05b merge.
- Metadata state read (record, locks, locked artwork, artwork rows): met
  (TestSaveMetadataRoundTripsAFullRecord, TestItemMetadataStateOfAnUnknownItem).
- Save replaces tags/people/provider IDs, " / " joined characters (helpers_test:65-80),
  case-insensitive person reuse, field locks keep stored values (one subtest per lockable
  field): met (TestSaveMetadataKeepsLockedFields).
- Artwork: local/embedded replaced, provider/upload kept, locked kind kept, one selection per
  kind, ID stability, embedded ownership by library: met (artwork tests listed in file).
- Orphan tags/people cleanup, cascade on delete: met
  (TestSaveMetadataSharesTagsAndPeopleAndDropsUnusedOnes, TestDeletingAnItem*).
- Neutral errors (ErrNotFound/ErrInvalid, no SQL details): met
  (TestSaveMetadataErrorsHaveNoSQLDetails, Rejects* tests).

## Notes (non-blocking)
- uniqueAsConflict (library_catalog_metadata.go:406) is 0 % covered; several error branches
  60-80 %.
- library_catalog_stub.go is temporary (M3-05b), ignored.

## History
- Round 1: PASS
