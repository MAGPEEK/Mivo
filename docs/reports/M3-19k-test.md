# M3-19k test report

Round 1 (run 2 of 3 by count): PASS

Gates (all green): go vet; go test -race with coverage (total 94.5 %, thresholds satisfied via go-test-coverage);
golangci-lint run (0 issues) and fmt --diff clean; go mod tidy -diff clean (root + tools);
go generate, pnpm -C web gen, Docker sqlc 1.31.1 produce no diff beyond the working tree; Docker Linux race for
http/..., app/library, sqlite, cmd ok; pnpm -C web lint/test (99.6 % stmts)/build ok; go vet and golangci-lint with -tags webui clean.

Criteria: handlers + problem-code mapping (problem_test.go TestProblemForError, TestPublishedProblemCodes), manageguard tests
(local/remote/IPv4-mapped, lan_only on/off, Origin, Sec-Fetch-Site, content type, exact route set, reads unaffected),
503 without manager (TestManagementWithoutManagerIsUnavailable), item counts (sqlite library_store_counts_test.go): met.
