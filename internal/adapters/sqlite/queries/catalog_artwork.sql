-- Write side of the artwork rows of an item (ADR 0027 section 8, M3-05d).

-- ListItemArtwork returns an item's artwork with the relative path of the media file an
-- embedded picture is read from.
-- name: ListItemArtwork :many
SELECT artwork.id, artwork.kind, artwork.source, artwork.relative_path, artwork.stream_index,
       artwork.data_path, artwork.size_bytes, artwork.modified_at, artwork.tag, artwork.selected,
       media_files.relative_path AS media_relative_path
FROM artwork
LEFT JOIN media_files ON media_files.id = artwork.media_file_id
WHERE artwork.item_id = ?
ORDER BY artwork.kind, artwork.id;

-- FindMediaFileID finds a media file by its place in a library.
-- name: FindMediaFileID :one
SELECT id
FROM media_files
WHERE library_id = ? AND relative_path = ?;

-- name: InsertArtwork :exec
INSERT INTO artwork (id, item_id, kind, source, library_id, relative_path, media_file_id,
                     stream_index, size_bytes, modified_at, tag, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateArtworkFile :exec
UPDATE artwork
SET size_bytes = ?, modified_at = ?, tag = ?
WHERE id = ?;

-- name: DeleteArtwork :exec
DELETE FROM artwork
WHERE id = ?;

-- name: ClearSelectedArtwork :exec
UPDATE artwork
SET selected = 0
WHERE item_id = ? AND kind = ?;

-- name: SelectArtwork :exec
UPDATE artwork
SET selected = 1
WHERE id = ?;
