# M6-05 test report

Status: PASS (round 1 of 3)

## Gates
- go vet ./...: clean
- go test -race ./internal/platform/... ./cmd/...: all ok (config 99.1 %, cmd/mivo 87.1 %)
- go test -race -coverprofile ./... : no failures
- golangci-lint run --allow-parallel-runners: 0 issues
- golangci-lint fmt --diff: clean
- go-test-coverage: total 94.7 % (>= 70 %), PASS; domain untouched
- Web/E2E/Docker: not applicable (platform/config + docs only)

## Acceptance criteria
- Keys + MIVO_ env (online.enabled=true, tmdb.enabled, tmdb.api_key, musicbrainz.enabled, language=en): met (metadata.go; precedence table in metadata_test.go:20-46). Docs in docs/development.md and ADR 0018.
- BundledTMDBKey via ldflags, empty in dev, TMDBKey() returns (key, origin none|bundled|own), own wins: met (TestTMDBKeyOrigin, TestOwnKeyFromFileWinsOverBundled). Verified with a real `go build -ldflags "-X ...BundledTMDBKey=..."`: the value is embedded in the binary.
- Key never printed: met. Secret type redacts via String/GoString/MarshalText/MarshalJSON/LogValue. TestKeyNeverAppearsInDumps covers slog JSON and text handlers, json.Marshal, %v, %+v, %#v and %s. Invalid-value error does not echo the key (TestMetadataInvalidValueNeverEchoesKey).
- Release docs (ldflags, CI secret, Dockerfile/SPK work without key): met in docs/development.md.
- Table tests for file < env, own > bundled, redaction: met.
- Lint, layer rules, coverage: met.

## Notes (non-blocking)
- Secret redaction relies on the type; TMDBKey() returns a plain string, so callers must not log it (documented in the doc comment).

## History
- Round 1: PASS
