-- Queries of the enrichment's metadata store (M6-14, ADR 0031): the persisted metadata
-- settings, the local source records, the item snapshot, match changes, provider artwork and
-- the batch enqueue after a scan.

-- name: GetMetadataSettings :one
SELECT online, tmdb, musicbrainz, language, tmdb_api_key
FROM metadata_settings
WHERE id = 1;

-- name: UpsertMetadataSettings :exec
INSERT INTO metadata_settings (id, online, tmdb, musicbrainz, language, tmdb_api_key, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE
SET online = excluded.online, tmdb = excluded.tmdb, musicbrainz = excluded.musicbrainz,
    language = excluded.language, tmdb_api_key = excluded.tmdb_api_key, updated_at = excluded.updated_at;

-- name: GetItemLocalRecords :one
SELECT nfo, filename
FROM item_local_records
WHERE item_id = ?;

-- UpsertItemLocalRecords replaces both records.
-- name: UpsertItemLocalRecords :exec
INSERT INTO item_local_records (item_id, nfo, filename, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (item_id) DO UPDATE
SET nfo = excluded.nfo, filename = excluded.filename, updated_at = excluded.updated_at;

-- UpsertItemLocalFilename replaces only the file name record; the NFO record is the given one
-- for a new row and stays for an existing one (an unreadable NFO must not wipe a good one).
-- name: UpsertItemLocalFilename :exec
INSERT INTO item_local_records (item_id, nfo, filename, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (item_id) DO UPDATE
SET filename = excluded.filename, updated_at = excluded.updated_at;

-- InsertItemLocalRecordsIfMissing seeds the local records of an item scanned before they
-- were kept, before provider data overwrites its stored record.
-- name: InsertItemLocalRecordsIfMissing :exec
INSERT INTO item_local_records (item_id, nfo, filename, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT (item_id) DO NOTHING;

-- name: GetEnrichmentItem :one
SELECT items.id, items.kind, items.parent_id, items.number, items.title, items.metadata_language,
       libraries.metadata_language AS library_language, libraries.online_metadata
FROM items
JOIN libraries ON libraries.id = items.library_id
WHERE items.id = ?;

-- FindMusicBrainzID returns an ID of the provider from the item itself or, for an album or an
-- artist whose own rows have none, from the tracks below it (the music writer keeps the tag
-- IDs on the tracks too). The item's own row sorts first.
-- name: FindMusicBrainzID :one
SELECT ids.external_id,
       CAST(CASE WHEN items.id = sqlc.arg(item_id) THEN 0
                 WHEN items.parent_id = sqlc.arg(item_id) THEN 1
                 ELSE 2 END AS INTEGER) AS depth
FROM item_provider_ids AS ids
JOIN items ON items.id = ids.item_id
LEFT JOIN items AS parent ON parent.id = items.parent_id
WHERE ids.provider = sqlc.arg(provider)
  AND (items.id = sqlc.arg(item_id)
       OR items.parent_id = sqlc.arg(item_id)
       OR parent.parent_id = sqlc.arg(item_id))
ORDER BY depth, ids.item_id
LIMIT 1;

-- name: DeleteItemFieldLocks :exec
DELETE FROM item_field_locks
WHERE item_id = ?;

-- name: InsertItemFieldLock :exec
INSERT INTO item_field_locks (item_id, field)
VALUES (?, ?)
ON CONFLICT (item_id, field) DO NOTHING;

-- name: InsertProviderArtwork :exec
INSERT INTO artwork (id, item_id, kind, source, data_path, provider, remote_url, size_bytes, modified_at,
                     tag, created_at)
VALUES (?, ?, ?, 'provider', ?, ?, ?, ?, ?, ?, ?);

-- ListShowDescendants returns the seasons of a show and the episodes below them or directly
-- below the show.
-- name: ListShowDescendants :many
SELECT child.id
FROM items AS child
WHERE child.kind IN ('season', 'episode')
  AND (child.parent_id = sqlc.arg(show_id)
       OR child.parent_id IN (SELECT season.id FROM items AS season
                              WHERE season.parent_id = sqlc.arg(show_id) AND season.kind = 'season'))
ORDER BY child.id;

-- name: GetLibraryOnlineMetadata :one
SELECT online_metadata
FROM libraries
WHERE id = ?;

-- ListMatchableItems returns the items of a library a provider can match (ADR 0031, hosts)
-- whose match is not disabled.
-- name: ListMatchableItems :many
SELECT items.id
FROM items
LEFT JOIN item_match ON item_match.item_id = items.id
WHERE items.library_id = ?
  AND items.kind IN ('movie', 'show', 'season', 'episode', 'artist', 'album')
  AND ifnull(item_match.state, 'unmatched') <> 'disabled'
ORDER BY items.id;

-- ListEnqueueableItems returns, with their kinds, those of the listed items whose library has
-- online metadata and whose match is not disabled; the caller filters the kinds.
-- name: ListEnqueueableItems :many
SELECT items.id, items.kind
FROM items
JOIN libraries ON libraries.id = items.library_id
LEFT JOIN item_match ON item_match.item_id = items.id
WHERE libraries.online_metadata = 1
  AND (item_match.state IS NULL OR item_match.state <> 'disabled')
  AND items.id IN (sqlc.slice(item_ids))
ORDER BY items.id;

-- RetryFailedEnrichment makes every job that failed before and is not running due now.
-- name: RetryFailedEnrichment :execrows
UPDATE enrichment_queue
SET not_before = ?
WHERE attempts > 0 AND claimed_until IS NULL AND not_before > ?;
