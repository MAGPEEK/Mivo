# M3-16e test report (round 2, counted as run 3) - PASS

- go vet ./...: clean
- go test -race -coverprofile ./...: all ok; go-test-coverage: total 94.5 % (threshold 70 %) PASS; iso package 92.3 %
- golangci-lint run: 0 issues; golangci-lint fmt --diff: no diff; gofmt -l clean
- go mod tidy -diff: clean (no go.mod change)
- GOARCH=386 go test ./internal/adapters/disc/...: ok
- Docker golang:1.27.3 go test -race -count=3 ./internal/adapters/disc/...: ok
- FuzzOpen 45 s: PASS, 14,814,439 execs, 236 corpus entries, no crashers, no testdata/fuzz files written (nothing to delete)

## Criteria
- UDF + ISO9660 fallback, ReaderAt files, multi-extent: met (iso tests)
- fs.FS view for resolvers: met
- Bounded memory, malformed errors, fuzz: met (FuzzOpen)
- In-process builder tests: met
- Gates, no new go.mod requirement: met

## History
- Round 1: PASS (30 s fuzz)
- Round 2: PASS (this report; earlier aborted run counted)
