# M3-13c test report (round 1)
status: PASS

Gates: go vet ./... clean; go test -race ./internal/adapters/ffmpeg/... ok; go test -race -coverprofile ./... ok;
go-test-coverage: total 94.6 % (>= 70), thresholds satisfied; golangci-lint run: 0 issues; golangci-lint fmt --diff: clean.
New integration test TestReadAudioTagsAndExtractCoverWithFFmpeg ran (not skipped): stream-tags.opus, plain.mp3, plain.flac, cover.flac, cover.mp3, cover.m4a PASS.
Other pre-existing corpus tests in the package SKIP (corpus not generated); unrelated to this task.

Criteria: all met. Tags/duration, ErrNoCover (wraps ErrNotFound), 16 MiB cap (TestCopyCoverRefusesOversizedOutput),
whitelist (TestAudioReadersRefuseADemuxerOutsideTheWhitelist), cancellation (TestAudioReadersHonourCancellation),
relative paths rejected, timeouts (coverTimeout 30s, probeTimeout), errors use filepath.Base (no absolute paths).

History: round 1 PASS.
