# M3-11 test report

Status: PASS (round 2)

## Gates
- go vet ./...: clean
- go test -race -coverprofile ./...: all ok; go-test-coverage PASS, total 95.5 % (>= 70 %); no domain function listed below 100 %
- golangci-lint run (windows + GOOS=linux for watcher/scantrigger): 0 issues; golangci-lint fmt --diff: clean
- go mod tidy -diff (root + tools): clean; govulncheck: no vulnerabilities
- -count=5 -race watcher + scantrigger: Windows ok (17.9 s / 1.7 s); Docker golang:1.27.3 ok (16.2 s / 1.1 s)
- config tests ok; cmd/mivo untouched
- GOOS=windows go build -gcflags=-m: notifier_windows.go:79 `new(windows.Overlapped) escapes to heap`, `new([65536]byte) escapes to heap`

## Acceptance criteria
All met: config keys scan.watch/scan.interval (config tests, ADR 0018 notes), watcher adapter, scantrigger use case, ports, tests (fake clock/watcher, temp dir on Windows and Linux), gates.

## Note (non-blocking)
Windows notifier (notifier_windows.go) has several functions at 66-81 % coverage and two at 0 % (lines 139, 283), error paths only; total thresholds met.

## History
- Round 1: see earlier run (not preserved)
- Round 2: PASS
