# M6-14 Wiring in `cmd/mivo`, scan trigger, periodic refresh
Owner: mivo-backend
Depends on: M6-05, M6-07, M6-08, M6-09, M6-10, M6-13
Files/areas: `cmd/mivo/**`, small hook in the scan use case (`internal/app/scan`)

## Goal
Everything runs together: after a scan, new items are enriched in the background; refresh and
identify work from the API; with online disabled the process opens no outbound socket.

## Acceptance criteria
- [ ] Composition root builds the outbound client, adapters, queue worker and scheduler and
      starts the worker with the app context; shutdown is clean.
- [ ] The scan enqueues changed/new items (`reason=scan`) via an `app` port; with
      `metadata.online.enabled=false` or `library.online_metadata=false` nothing is enqueued
      and no adapter is constructed with network access.
- [ ] Daily periodic `RefreshDue` runs at the existing scheduler cadence (no extra timer
      hosts besides ADR 0031 §1).
- [ ] Integration test: start the app with a fake TMDB `httptest` server (base URL option),
      scan the test corpus, observe `matched` items and provider artwork via the HTTP API.
- [ ] Integration test: same with the fake server stopped → scan completes, library usable,
      status `offline`.
- [ ] Network guard test: with online disabled, a dialing hook records zero non-loopback dials.
- [ ] Tests run with `go test -race ./cmd/... ./internal/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
`cmd/mivo` stays wiring only. Base-URL overrides are test options, not documented config.
