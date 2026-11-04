# M3-07 test report

Round 1 – status: PASS

## Gates
- go vet ./... : clean
- go test -race -coverprofile ./... : all ok
- go-test-coverage: total 96.2 % (>= 70 %), package gate satisfied; nfo package functions mostly 100 %, lowest 75 % (decode.go passThroughCharset/decodeWith/malformed, providers.go tmdbIDFromPath)
- golangci-lint run ./... --allow-parallel-runners : 0 issues
- golangci-lint fmt --diff : NOT RUN to completion (the fmt subcommand rejects --allow-parallel-runners and hung >120 s without the flag, probably a lock held by a parallel run). Substitute: `gofmt -l internal/adapters/metadata/nfo` prints nothing.
- go mod tidy -diff : clean
- govulncheck ./... : no vulnerabilities
- Layer rule: internal/domain deps contain no database/sql, net/http, os/exec, adapters or app.

## Acceptance criteria
- Parse API, kinds, Result shape, warnings: met (nfo.go, 100 % covered)
- ADR 0027 elements, plot/outline fallback, year fallback, default uniqueid, ID validation: met (fixtures outline_fallback, default_uniqueid, invalid_values)
- URL NFO / XML+URL: met (url_only_imdb/tmdb/tmdb_show/tvdb_query/tvdb_series/unknown, xml_with_url)
- Encoding: met (bom_utf8, latin1, windows1252, nfd_text, utf16_declared, invalid_utf8)
- Limits and malformed: met (malformed_*, external_entity, empty, whitespace_only, wrong_root)
- No fetch (bytes only): met; imports_test.go guards imports
- Fixtures >= 20: met, 34 files in internal/adapters/metadata/nfo/testdata
- go.mod: only x/text promoted to direct requirement: met (git diff)
- Gates: met except the fmt caveat above

## History
- Round 1: PASS
