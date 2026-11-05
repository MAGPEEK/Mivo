-- Online metadata queries (ADR 0031): match state, provider cache, enrichment queue and the
-- per-library switch.

-- GetItemMatch returns no state columns for an item without a match row, and no row at all
-- for an unknown item.
-- name: GetItemMatch :one
SELECT items.id, item_match.state, item_match.provider, item_match.confidence, item_match.matched_at,
       item_match.ref_provider, item_match.ref_kind, item_match.ref_id, item_match.checked_at
FROM items
LEFT JOIN item_match ON item_match.item_id = items.id
WHERE items.id = ?;

-- name: UpsertItemMatch :exec
INSERT INTO item_match (item_id, state, provider, confidence, matched_at, ref_provider, ref_kind, ref_id,
                        checked_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (item_id) DO UPDATE
SET state = excluded.state, provider = excluded.provider, confidence = excluded.confidence,
    matched_at = excluded.matched_at, ref_provider = excluded.ref_provider, ref_kind = excluded.ref_kind,
    ref_id = excluded.ref_id, checked_at = excluded.checked_at;

-- ListItemsDueForRefresh returns the items of libraries with online metadata whose last
-- enrichment was before the cutoff (or never recorded), oldest first; disabled matches are
-- skipped. Items already queued are skipped too: re-enqueueing them would pull a backed-off
-- job forward (Enqueue takes the earlier due time) and stuck items would fill every batch.
-- The first parameter is the cutoff, the second the limit.
-- name: ListItemsDueForRefresh :many
SELECT item_match.item_id
FROM item_match
JOIN items ON items.id = item_match.item_id
JOIN libraries ON libraries.id = items.library_id
WHERE item_match.state <> 'disabled'
  AND libraries.online_metadata = 1
  AND NOT EXISTS (SELECT 1 FROM enrichment_queue WHERE enrichment_queue.item_id = item_match.item_id)
  AND (item_match.checked_at IS NULL OR item_match.checked_at < ?)
ORDER BY item_match.checked_at, item_match.item_id
LIMIT ?;

-- name: GetProviderCache :one
SELECT payload, fetched_at, expires_at, not_found
FROM provider_cache
WHERE provider = ? AND cache_key = ?;

-- name: UpsertProviderCache :exec
INSERT INTO provider_cache (provider, cache_key, payload, fetched_at, expires_at, not_found)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, cache_key) DO UPDATE
SET payload = excluded.payload, fetched_at = excluded.fetched_at, expires_at = excluded.expires_at,
    not_found = excluded.not_found;

-- EnqueueEnrichment keeps one job per item: a repeated request takes the newer reason, the
-- earlier due time and a new revision; attempts and the lease stay.
-- name: EnqueueEnrichment :exec
INSERT INTO enrichment_queue (item_id, reason, not_before, enqueued_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (item_id) DO UPDATE
SET reason = excluded.reason, not_before = min(enrichment_queue.not_before, excluded.not_before),
    revision = enrichment_queue.revision + 1;

-- ClaimEnrichment leases the next due job that no live worker holds. The first parameter
-- is the lease end, the other two are the current time; the predicate keeps the range on
-- not_before so enrichment_queue_due serves it.
-- name: ClaimEnrichment :one
UPDATE enrichment_queue
SET claimed_until = ?
WHERE item_id = (SELECT queued.item_id
                 FROM enrichment_queue AS queued
                 WHERE queued.not_before <= ?
                   AND (queued.claimed_until IS NULL OR queued.claimed_until <= ?)
                 ORDER BY queued.not_before, queued.enqueued_at, queued.item_id
                 LIMIT 1)
RETURNING item_id, reason, not_before, attempts, last_error, revision, claimed_until;

-- The statements that finish a job match its lease, so a worker whose lease ran out and
-- whose job another worker claimed changes nothing.

-- name: DeleteEnrichmentRevision :execrows
DELETE FROM enrichment_queue
WHERE item_id = ? AND revision = ? AND claimed_until = ?;

-- ReleaseEnrichment makes a job claimable again right away; it serves a job that was
-- re-requested while it ran.
-- name: ReleaseEnrichment :execrows
UPDATE enrichment_queue
SET claimed_until = NULL, attempts = 0, last_error = ''
WHERE item_id = ? AND claimed_until = ?;

-- ReleaseEnrichmentClaim gives an interrupted job back without counting an attempt.
-- name: ReleaseEnrichmentClaim :execrows
UPDATE enrichment_queue
SET claimed_until = NULL
WHERE item_id = ? AND claimed_until = ?;

-- FailEnrichment keeps the earlier due time of a request that came in while the job ran.
-- name: FailEnrichment :execrows
UPDATE enrichment_queue
SET claimed_until = NULL, attempts = attempts + 1, last_error = ?,
    not_before = CASE WHEN revision = ? THEN ? ELSE not_before END
WHERE item_id = ? AND claimed_until = ?;

-- name: CountEnrichmentJobsOfItem :one
SELECT count(*)
FROM enrichment_queue
WHERE item_id = ?;

-- name: CountEnrichmentJobs :one
SELECT count(*)
FROM enrichment_queue;

-- DeleteProviderArtwork returns the stored files of the removed rows so the caller can
-- delete them from the data directory.
-- name: DeleteProviderArtwork :many
DELETE FROM artwork
WHERE item_id = ? AND source = 'provider'
RETURNING data_path;

-- name: GetItemOnlineMetadata :one
SELECT libraries.online_metadata
FROM items
JOIN libraries ON libraries.id = items.library_id
WHERE items.id = ?;

-- name: SetLibraryOnlineMetadata :execrows
UPDATE libraries
SET online_metadata = ?, updated_at = ?
WHERE id = ?;
