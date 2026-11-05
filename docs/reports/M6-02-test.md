# M6-02 test report

Result: PASS (run 2; run 1 aborted)

## Gates
- go vet ./...: clean
- go test -race -coverprofile ./...: all ok
- golangci-lint run: 0 issues; golangci-lint fmt --diff: clean
- go-test-coverage: PASS; total 94.7 % (>= 70); domain packages all >= 90 % (match 99.5 %, metadata 100 %, domain total 99.6 %)
- go list -deps ./internal/domain/...: no database/sql, net/http, os/exec, adapters, app

## Acceptance criteria
- Source tmdb/musicbrainz/user + documented order, Merge tests with tmdb and locked field: met (source.go, merge_test.go)
- MatchState + ParseMatchState (ErrInvalid): met (state.go)
- NormalizeTitle incl. German/CJK: met (normalize.go, match_test.go)
- Score in [0,1], exact ID 1, year factors 1/0.92/0.6, sequel-number cap 0.7 (numbers.go), subtitle penalty, 200-rune length cap (similarity.go): met
- Decide: threshold 0.85, lead 0.10, exact ID auto: met (decide.go); Resolve keeps matched/disabled/locked
- ParseProviderRef (TMDB movie/tv, IMDb tt, MusicBrainz UUID): met (ref.go), ErrInvalid otherwise
- Tests table-driven, coverage >= 90 %: met
- Gates: met

## History
- Round 1 aborted; round 2 PASS
