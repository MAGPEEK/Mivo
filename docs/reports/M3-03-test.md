# M3-03 test report (round 2) - PASS

| Gate | Result |
|---|---|
| go vet ./... | clean |
| go test -race -coverprofile ./... | all ok; naming 98.7 %, library 100 %, total 87.7 % |
| go-test-coverage (.testcoverage.yml) | PASS (total 96.2 %, 7386/7677) |
| golangci-lint run | 0 issues |
| golangci-lint fmt --diff | clean |
| go mod tidy -diff | clean |
| Layer rule (go list -deps naming) | no database/sql, net/http, os/exec, adapters, app |

## Corpus check (throwaway test, removed afterwards)
gen-testmedia ran; naming.Place over the 19 videos of testdata/library/movies vs manifest.json
(versions, labels, parts, extras type + parent, sample ignored): all match. Three title diffs
are not defects: Cosmos Laundromat and Das Boot manifest titles come from the NFO; Amelie is
NFD on disk while the parser contract takes NFC (the scan normalises).

## Acceptance criteria
- Place/FileContext/Result, Shows -> ErrUnsupported, other kind -> ErrInvalid: met (place.go:38)
- Tokenizer exported, years, folder movie/version label, parts, extras, samples: met (tests green, corpus matches)
- Table rows: 72 movie (>= 40), 20 extras/samples (>= 15): met
- Coverage >= 90 % (naming 98.7 %), gates: met

## History
- Round 1: see earlier run (overwritten). Round 2: PASS.
