# M3-19f test report

Result: PASS (run 2 of 3; run 1 aborted)

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... (host) | all ok |
| go-test-coverage | total 94.6 % (>= 70), PASS |
| golangci-lint run / fmt --diff | 0 issues / no diff |
| go mod tidy -diff (root + tools) | clean |
| sqlc 1.31.1 generate (Docker) | sqlcgen unchanged (hash identical), no untracked generated files |
| Docker Linux race, sqlite + cmd | ok (36.5 s / 2.0 s) |
| Migration up/down/carry-over | TestMigrateCarriesLibrariesOverToFoldersAndTheImportLedger, TestMigrationDownToVersion3/2..., TestMigrationsCanBeRolledBackAndReapplied PASS |
| CHECK/unique table test, folder cascade | TestLibraryManagementConstraints{Reject,Accept}..., TestDeletingAFolderCascadesToItsCatalogRows PASS |
| Windows re-key fix-up | TestRekeyFolders*, TestSyncLibrariesMatchesRootsByFolderKey PASS |

Criteria: all met. root_path removed from libraries (libraries rebuilt); no query reads the legacy column
(only aliased from library_roots.path). Composite FK deliberately absent, documented in the migration header
(invariant) and BACKLOG for M3-19h. Domain layer check not affected by this task (no domain files changed).

History: round 2 PASS.
