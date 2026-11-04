# M3-19i test report (run 3 of 3) - FAIL

Passing: go vet; go test -race ./... (coverage 94.7 %, go-test-coverage PASS, total >= 70 %); golangci-lint run 0 issues; fmt --diff clean; go mod tidy -diff clean (root + tools); corpustest not skipped, PASS.

## Failure: flaky test (Docker Linux, `go test -race -count=20 ./internal/app/scan/...`)
- internal/app/scan/service_test.go:266 `TestCloseCancelsARunningScanWithoutRecordingFailures`
- `after Close: 0 probes active, 3 canceled, want 0 and 2`
- Cause: the test waits for only 2 `entered` signals, then calls Close. Between the second receive and Close, a worker can free a slot and start a third probe (queue holds 4 files), which is also canceled, so canceled == 3 instead of MaxConcurrentProbes (2). The assertion must not depend on exactly 2; e.g. use `canceled >= MaxConcurrentProbes`, or make the fake block new entries until Close.
- The sqlite/cmd -race run was not reached in the same Docker command (&& chain stopped); rerun after the fix.

Acceptance criteria: not re-verified this round (only test code changed); the verdict is FAIL because of the flaky test.
