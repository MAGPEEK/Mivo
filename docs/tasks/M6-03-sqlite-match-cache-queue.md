# M6-03 SQLite: migration 00008 (00004 is library management; 00005 music, 00006 audiobooks, 00007 photos are reserved; take the next free number at merge), match state, provider cache, enrichment queue
Owner: mivo-backend
Depends on: M3-05d, M6-02
Files/areas: `internal/adapters/sqlite/**` (migrations, queries, sqlc output, new store files)

## Goal
Persist what online enrichment needs: per-item match state, a response cache so offline works,
and a resumable queue.

## Acceptance criteria
- [ ] Migration `00008` (next free number at merge; 00004 is taken by library management) adds `item_match(item_id, state, provider, confidence, matched_at)`,
      `provider_cache(provider, cache_key, payload, fetched_at, expires_at, not_found)`,
      `enrichment_queue(item_id, reason, not_before, attempts, last_error)` and
      `libraries.online_metadata` (default 1); existing data migrates unchanged.
- [ ] Store methods: get/set match state, read/write cache (stale read allowed), enqueue
      (dedupe per item), claim next due job, mark done/failed with `not_before` backoff,
      delete provider artwork rows of an item.
- [ ] A queue survives reopening the database; a claimed job whose worker died is re-claimable
      after a lease time.
- [ ] `catalog.Writer` still refuses locked fields when provider data is written (regression).
- [ ] Tests: integration tests on real SQLite in a temp dir incl. migration up from `00003`
      with data; `sqlc generate` leaves `git diff --exit-code` clean.
- [ ] Gates: lint + layer rules + coverage thresholds pass.

## Notes
Payloads are provider-neutral JSON produced in `app`/adapters, never raw provider responses
containing keys. Use ports declared in M6-06/M6-11; implement them here without importing `app`
from the wrong direction (adapters import `app` interfaces only).
