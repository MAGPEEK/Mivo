-- Online metadata (ADR 0031): per-item match state, a provider response cache that keeps
-- Mivo working offline, a persisted enrichment queue and the per-library switch. ADR 0004
-- conventions apply: timestamps are INTEGER Unix milliseconds in UTC, every table is STRICT.
-- Existing rows are untouched; every library starts with online_metadata = 1 (ADR 0031 §2).

-- +goose Up
ALTER TABLE libraries ADD COLUMN online_metadata INTEGER NOT NULL DEFAULT 1 CHECK (online_metadata IN (0, 1));

-- An item without a row is unmatched. A match names its provider and when it was made;
-- confidence is the score of match.Decide (1 for a user identification).
CREATE TABLE item_match (
    item_id    TEXT    NOT NULL PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    state      TEXT    NOT NULL CHECK (state IN ('unmatched', 'matched', 'disabled')),
    provider   TEXT    NOT NULL DEFAULT '',
    confidence REAL    NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
    matched_at INTEGER,
    CHECK (state <> 'matched' OR (provider <> '' AND matched_at IS NOT NULL))
) STRICT;

-- Provider-neutral JSON produced by the adapters, never a raw response with keys in it. A
-- "not found" answer is cached too (empty payload) so it is not asked again before it
-- expires. Expired rows stay readable: they are the offline fallback.
CREATE TABLE provider_cache (
    provider   TEXT    NOT NULL CHECK (provider <> ''),
    cache_key  TEXT    NOT NULL CHECK (cache_key <> ''),
    payload    TEXT    NOT NULL CHECK (payload = '' OR json_valid(payload)),
    fetched_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL CHECK (expires_at >= fetched_at),
    not_found  INTEGER NOT NULL CHECK (not_found IN (0, 1)),
    CHECK ((not_found = 1) = (payload = '')),
    PRIMARY KEY (provider, cache_key)
) STRICT;

-- One job per item. claimed_until is the lease of the worker running it: a job whose worker
-- died becomes claimable again once the lease ran out. revision grows with every enqueue, so
-- finishing a job that was re-requested while it ran keeps the newer request.
CREATE TABLE enrichment_queue (
    item_id       TEXT    NOT NULL PRIMARY KEY REFERENCES items (id) ON DELETE CASCADE,
    reason        TEXT    NOT NULL CHECK (reason IN ('scan', 'refresh', 'identify', 'periodic')),
    not_before    INTEGER NOT NULL,
    attempts      INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error    TEXT    NOT NULL DEFAULT '',
    enqueued_at   INTEGER NOT NULL,
    revision      INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    claimed_until INTEGER
) STRICT;

-- Due jobs in claim order.
CREATE INDEX enrichment_queue_due ON enrichment_queue (not_before, enqueued_at, item_id);

-- +goose Down
DROP INDEX enrichment_queue_due;
DROP TABLE enrichment_queue;
DROP TABLE provider_cache;
DROP TABLE item_match;
ALTER TABLE libraries DROP COLUMN online_metadata;
