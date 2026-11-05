# M3-16c test report (run 2 of 3, round 2)

Status: PASS

Gates (all in worktree, uncommitted tree):
- go vet ./...: clean
- go test ./...: all ok; go test -race ./internal/adapters/disc/...: ok
- golangci-lint run --allow-parallel-runners: 0 issues; golangci-lint fmt --diff: clean
- go mod tidy -diff: clean (no new go.mod requirement)
- go-test-coverage: PASS, total 94.6 %; disc functions 100 % except iso/file.go:114 dirFile.Read (0 %, trivial error path)
- GOARCH=386 vet + test disc/...: ok
- Docker golang:1.27.3 -race -count=3 disc/...: ok
- Fuzz 20 s each (-fuzzminimizetime 0), no crashers: FuzzParsePlaylist 14,352,626 execs; FuzzParseTitleSetDuration 4,557,776; FuzzResolveTreeBDMV 8,878,894; FuzzOpen (iso) 6,110,418. No testdata/fuzz corpus left in tree.

Acceptance: BDMV/VIDEO_TS selection, disc_no_title for missing clips/unparsable playlists (bdmv.go:60, videots.go:64, image.go:101), traversal-safe tree access, synthetic fixtures + fuzz tests, gates: met.
Mapping: corrupt ISO listings/extents -> ErrDiscUnsupported via classifyImageError (image.go:26-36); cancellation/deadline and already-coded errors are not tagged (image.go:30-31, tree.go:170); tests present in image_test.go.

History: round 1 aborted; round 2 PASS.
