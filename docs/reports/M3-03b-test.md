# M3-03b test report: name parser for shows, seasons, episodes

Status: PASS (round 2; the earlier aborted run counted as round 1)

## Gates

| Gate | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race -coverprofile=coverage.out ./...` | all packages ok |
| go-test-coverage (`.testcoverage.yml`) | PASS, total 95.8 % (threshold 70 %) |
| coverage `internal/domain/naming` | 99.1 % (threshold 90 %); other domain packages 100 % |
| `golangci-lint run` | 0 issues |
| `golangci-lint fmt --diff` | no diff |
| `go mod tidy -diff`, `go -C tools mod tidy -diff` | no diff |
| layer rule (`go list -deps ./internal/domain/...`) | no database/sql, net/http, os/exec, adapters, app |

Web, Docker and E2E gates: not applicable (domain-only task).

## Acceptance criteria

- Placement kinds, show/season/episode/extra/ignored, markers, absolute numbering: met. They are covered by about 63 table rows in `show_test.go` and `show_edge_test.go`, which is at least the 50 required.
- Regression: `TestAbsoluteNumberingDragonBallSuper` and `TestAbsoluteNumberingVariants` pass. They assert 1 show, 1 season, 131 episodes and no season above 1.
- Tests and coverage: met (see Gates).

## M3-17 library comparison (naming.Place vs testdata/library/manifest.json)

`go run ./scripts/gen-testmedia` was run. A throwaway program, deleted afterwards, called `naming.Place(KindShows, ...)` for all 138 video files below `testdata/library/shows`. It compared show title, season, episode, numberEnd and absoluteNumber with the manifest.

- 138 files, 138 manifest entries, 0 mismatches.
- Dragon Ball Super (DT-24): 1 show, 1 season, 131 episodes.
- Dragon Ball Super Fansub: 1 season, 1 episode (E130, abs 130; `[Group]` prefix, `[1080p]` and `[ABCD1234]` ignored).
- Pioneer One (2010): 3 seasons (Specials is 0, Season 01, Season 02), 4 episodes. `S01E01-E02` is episode 1 with end 2.
- Star Wreck, root files `Star Wreck - S01E01/E02.mkv`: 1 season, 2 episodes, show key `name:star wreck`.

## History

- Round 1: aborted, counts as a round.
- Round 2: PASS.
