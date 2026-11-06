# M7-23 HTTP: watch handlers, viewer state, row kinds
Owner: mivo-backend
Depends on: M7-03, M7-19, M7-20
Files/areas: `internal/adapters/http/restapi/watch*.go` (replaces `watch_stub.go`), item summary/detail response mapping, `restapi/rows.go`, one field each in `restapi/api.go` and `restapi/options.go`

## Goal
The M7-03 operations work, item lists and details carry `viewerState`, and the new row kinds
reach the clients.

## Acceptance criteria
- [ ] Progress, watched (PUT/DELETE) and continue-watching hide map to `app/watch`;
      `400 invalid` for unsupported kinds, `404` for unknown or hidden items.
- [ ] Item listings, children, rows, search results and item detail add `viewerState` from
      `app/watch.States` (one batch per response, not per item).
- [ ] `RowKind` mapping covers `continueWatching` and `upNext`; the contract-values test that
      pins enum parity is updated.
- [ ] Tests: handler tests with fakes (`go test ./internal/adapters/http/restapi/ -run 'Watch|Rows|Items'`),
      including a page of 100 items calling `States` once.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
If search results are mapped elsewhere (M5-15), add the decoration there in the same way.
