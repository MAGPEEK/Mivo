-- Write side of the merged item metadata (ADR 0027 section 6, M3-05d).

-- name: UpdateItemMetadata :execrows
UPDATE items
SET title          = ?,
    sort_title     = ?,
    original_title = ?,
    year           = ?,
    release_date   = ?,
    summary        = ?,
    tagline        = ?,
    content_rating = ?,
    updated_at     = ?
WHERE id = ?;

-- name: ListItemFieldLocks :many
SELECT field
FROM item_field_locks
WHERE item_id = ?
ORDER BY field;

-- name: DeleteItemTags :exec
DELETE FROM item_tags
WHERE item_id = ?;

-- name: DeleteItemPeople :exec
DELETE FROM item_people
WHERE item_id = ?;

-- DeleteItemProviderIDs removes the provider IDs the metadata record owns. The MusicBrainz
-- IDs of the audio tags (providers musicbrainz_*) belong to the music writer and stay.
-- name: DeleteItemProviderIDs :exec
DELETE FROM item_provider_ids
WHERE item_id = ?
  AND provider NOT LIKE 'musicbrainz\_%' ESCAPE '\';

-- FindTagID matches the name case-insensitively, so "Sci-Fi" and "sci-fi" share one tag.
-- name: FindTagID :one
SELECT id
FROM tags
WHERE kind = ? AND name = ? COLLATE NOCASE;

-- name: InsertTag :exec
INSERT INTO tags (id, kind, name)
VALUES (?, ?, ?);

-- name: InsertItemTag :exec
INSERT INTO item_tags (item_id, tag_id, position)
VALUES (?, ?, ?);

-- name: FindPersonID :one
SELECT id
FROM people
WHERE name = ? COLLATE NOCASE;

-- name: InsertPerson :exec
INSERT INTO people (id, name, sort_name)
VALUES (?, ?, ?);

-- name: InsertItemPerson :exec
INSERT INTO item_people (item_id, person_id, role, character, position)
VALUES (?, ?, ?, ?, ?);

-- name: InsertItemProviderID :exec
INSERT INTO item_provider_ids (item_id, provider, external_id)
VALUES (?, ?, ?);

-- DeleteUnusedTags removes tags no item references any more.
-- name: DeleteUnusedTags :exec
DELETE FROM tags
WHERE NOT EXISTS (SELECT 1 FROM item_tags WHERE item_tags.tag_id = tags.id);

-- DeleteUnusedPeople removes people no item references any more.
-- name: DeleteUnusedPeople :exec
DELETE FROM people
WHERE NOT EXISTS (SELECT 1 FROM item_people WHERE item_people.person_id = people.id);
