# M6-11 App: enrichment queue, cache, offline behaviour
Owner: mivo-backend
Depends on: M6-03, M6-06
Files/areas: `internal/app/enrich/queue*.go`, `internal/app/enrich/*_test.go`

## Goal
A background worker that enriches queued items without ever blocking a scan or the UI, and
that behaves sensibly offline, rate-limited or with a bad key.

## Acceptance criteria
- [ ] `Queue.Enqueue(itemID, reason)` (reasons `scan|refresh|identify|periodic`) dedupes; a
      scan only enqueues new/changed items and never waits.
- [ ] Worker with bounded concurrency (default 2) claims due jobs, calls `Enricher`; success
      removes the job.
- [ ] `ErrUnavailable` → job rescheduled with exponential backoff (cap 6 h), status `offline`;
      `ErrRateLimited` → `not_before` from `Retry-After`; invalid key → provider state
      `disabled` until settings change, no retry storm.
- [ ] Offline scenario: with a fake provider that always fails, a full library of items keeps
      local data, `/metadata/status` reports `offline`, nothing errors to the caller.
- [ ] Cache: provider responses read through `ProviderCache` (TTL 7 d, not-found 30 d); stale
      entries are served when the provider is unreachable.
- [ ] Graceful shutdown: running jobs get the context cancelled, are re-queued; restart resumes.
- [ ] Periodic refresh: `Scheduler.RefreshDue(now)` enqueues items whose cache expired,
      skipping `disabled`; clock injected.
- [ ] Tests: fakes and a fake clock; `go test -race ./internal/app/enrich/...`.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Metrics for status: queue length, last success, per-provider state. No goroutine started in
constructors; `Run(ctx)` is called from `cmd/mivo`.
