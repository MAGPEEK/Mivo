# M6-03 test report

Status: FAIL (round 1 of 3)

## Gates
| Gate | Result |
|---|---|
| go vet ./... | pass |
| go test -race ./... (Windows) | pass |
| go test -race sqlite + cmd (Docker Linux, golang:1.27.3) | pass (final run) |
| golangci-lint run --allow-parallel-runners | FAIL |
| golangci-lint fmt --diff | clean |
| go-test-coverage | pass, total 94.7 % (>= 70) |
| sqlc generate (Docker 1.31.1) | idempotent, generated files match the worktree |
| Layer rules | enforced by depguard in golangci-lint (no depguard finding) |

## Failure
- internal/adapters/sqlite/enrichment_queue_test.go:296 `TestConcurrentClaimersGetEveryJobOnce`:
  `cognitive complexity 17 of func ... is high (> 15) (gocognit)`. Split the worker loop into a helper.

Note: during the run the implementer's test file changed (the lease assertion in
`TestEnqueueKeepsOneJobPerItemWithTheNewestReason` first failed on Linux with lease 12:05:01 vs
12:05:00 because of `advance(time.Second)`; fixed in the tree before the final runs, which are green).

## Acceptance criteria
- Migration 00005 (item_match, provider_cache, enrichment_queue, libraries.online_metadata default 1), up/down, carry-over from version 3 and data kept: met (online_metadata_schema_test.go: ...KeepsExistingData, ...FromVersion3..., ...DownToVersion4..., constraints).
- Store: match get/set, cache read/write incl. expired entries and not_found, enqueue dedupe with newest reason, claim, done/failed with backoff, revision protection, delete provider artwork, cascade on item delete: met (provider_cache_test.go, enrichment_store_test.go, enrichment_queue_test.go, schema cascade test).
- Queue survives reopen; expired lease re-claimable; stale worker cannot finish: met (TestQueueSurvivesReopeningAndExpiredLeasesAreReclaimable, TestStaleWorker...).
- catalog.Writer locked-field regression: met (TestProviderDataForAMatchedItemKeepsLockedFields).
- sqlc diff clean: met (generation idempotent; new files untracked/uncommitted as expected).
- Gates lint: NOT met (see above).

## History
- Round 1: FAIL, gocognit.
