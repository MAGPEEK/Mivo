# M3-16k Composition root: disc wiring and integration test
Owner: mivo-backend
Depends on: M3-16h, M3-16i, M3-16j (M3-16g optional: ISO)
Files/areas: `cmd/mivo`, integration test

## Goal
Discs work end to end on the server in movie and show libraries.

## Acceptance criteria
- [ ] Resolver, encryption check, prober and (when merged) ISO source are wired; no new library kind or tile.
- [ ] Integration test over the disc corpus: VIDEO_TS and BDMV movies appear with duration and streams, the encrypted stand-in is `unavailable` with its code, a session starts and delivers a first segment.
- [ ] Tests: `go test -race -tags webui ./cmd/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
No web task: unavailable items already render through the generic "unavailable" state; if not, add a one-line badge in M3-16k only.
