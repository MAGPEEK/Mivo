# M3-16g test report

Round 1: PASS (commit 47ccda2)

- golangci-lint run, fmt --diff, go vet, go mod tidy -diff: clean
- go test -race -coverprofile ./...: all ok; total 83.5% (go-test-coverage 94.6%, threshold 70%); domain packages 99-100%
- Layer rule: go list -deps ./internal/domain/... shows no database/sql, net/http, os/exec, adapters, app
- Corpus regenerated; corpustest ok incl. TestCorpusDiscTitlesProbe (ISO covered), TestCorpusVideosPlayWithTheExpectedMode (single-file regression)
- ffmpeg/disc/iso integration tests native and in Docker golang:1.27.3 + apt ffmpeg with -race (Linux fd branch): ok, only Windows-only skips
- Whitelists: allowedProtocols = "file"; image adds only subfile (+concat for DVD); no network protocol
- Acceptance criteria: all met
