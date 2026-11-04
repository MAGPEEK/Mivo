# M3-10 test report (run 3)

Status: PASS

## Gates
- go vet ./...: clean
- go test -race -coverprofile ./...: all ok
- go-test-coverage: PASS, total 95.3 % (threshold 70 %); internal/domain/* 99-100 %
- golangci-lint run: 0 issues
- golangci-lint fmt --diff: empty
- go mod tidy -diff: clean
- Docker Linux race run (golang:1.27.3) internal/app/scan: ok
- M2 regression: gen-testmedia + corpustest integration test ok (FFmpeg present)
- Layer rule: golangci-lint depguard clean; domain untouched by this change

## Acceptance criteria
- CatalogV2 port (split per review deviation), dual path, Finished on both paths: met (TestV2DefaultsAndFinished, TestM2PathCallsFinished, TestV2ValidationNeedsACatalog)
- Fake enforces contract, table-tested: met (TestMemoryCatalogV2Contract)
- Movie versions + trailer, extras without parent skipped, rescan probes/moves nothing: met
- Rename keeps item id (TestV2RenamedEpisodeKeepsItsID), folder rename moves without probe and removes old show: met
- DT-24 131 absolute-numbered files, 1 show, 1 season, 131 episodes: met (TestV2AbsoluteNumberedShowIsOneShowOneSeason; corpus also reports 131 episodes)
- root_unavailable / skipped-path safety (TestV2SafetyRules, TestV2OrphanExtraIsKept..., cancel test): met
- Refresh set incl. sidecar changes; early end still refreshes touched items with walked sidecars: met (TestV2RefreshSetCoversSidecarChanges, TestV2EarlyEndStillRefreshesTouchedItems, TestRefreshBelowSidecars*)
- BACKLOG rows updated: BACKLOG.md modified
- Gates listed in task: met

## Findings (non-blocking)
- run_v2.go:252 record 33.3 % and run_v2.go:278 recordNoVideo 0 % covered (probe-failed and no-video-stream branches); no threshold on it, consider a test.

## History
- Round 3: PASS
