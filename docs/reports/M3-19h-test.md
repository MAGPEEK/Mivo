# M3-19h test report (round 1 of 3)

Status: PASS

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok |
| go-test-coverage | total 94.6 % (>= 70 %), thresholds satisfied |
| golangci-lint run / fmt --diff | 0 issues / no diff |
| go mod tidy -diff (root, tools) | clean |
| Docker sqlc generate | no change vs. working tree (incl. untracked) |
| Docker Linux race, sqlite + scan -count=3 | ok (sqlite 102 s, scan 1.7 s) |
| Docker Linux race, cmd/... | ok |
| corpustest (testdata copied, removed afterwards) | PASS, not skipped |

## Criteria
- Writers store RootID (placement, sidecars, artwork): met (catalog_placement.go:177/227, catalog_sidecars.go, library_catalog_artwork.go).
- Foreign/unknown/empty root -> domain.ErrInvalid via requireLibraryRoot; TestWritersRefuseAFolderOfAnotherLibrary: met.
- Readers return RootID (files, sidecars, artwork incl. media_root_id): met.
- FindMediaFileID WHERE root_id; DeleteSidecarState by library_id, root_id, path: met.
- Ordering part_number, library_roots.position, relative_path in ListPrimaryFiles and ListItemFiles: met (TestFoldersOrderThePrimaryFileAndResolveTheirPaths).
- Same relative path in two folders, per-folder removal, per-folder artwork tests present and green: met.
- Empty-RootID fallback and canonicalRootID removed from scan: met (folders.go, run_v2.go).
- sqlc output committed, regenerate clean: met.

Note: an earlier coverage.out was removed; no untracked files other than catalog_multi_folder_test.go.
