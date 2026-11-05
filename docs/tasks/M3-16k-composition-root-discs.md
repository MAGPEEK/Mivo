# M3-16k Composition root: disc wiring and integration test
Owner: mivo-backend
Depends on: M3-16h, M3-16i, M3-16j (M3-16g optional: ISO)
Files/areas: `cmd/mivo`, integration test

## Goal
Discs work end to end on the server in movie and show libraries.

## Acceptance criteria
- [x] Resolver, encryption check, prober and (when merged) ISO source are wired; no new library kind or tile.
- [x] Integration test over the disc corpus: VIDEO_TS and BDMV movies appear with duration and streams, the encrypted stand-in is `unavailable` with its code, a session starts and delivers a first segment.
- [x] Session API over the disc corpus (`testdata/library/discs`, M3-16i): `Movie (2019)/VIDEO_TS` plays to the end through a session (assert in `internal/adapters/corpustest` next to `TestCorpusDVDTitlePlays`, which today stops at the producer).
- [x] `scripts/playback-smoke` gets one profile row that plays the VIDEO_TS title through the session API (left open by M3-16i because the server could not reach discs yet).
- [x] Tests: `go test -race -tags webui ./cmd/...`.
- [x] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No web task: unavailable items already render through the generic "unavailable" state; if not, add a one-line badge in M3-16k only.
