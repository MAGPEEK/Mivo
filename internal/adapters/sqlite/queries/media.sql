-- ListFileStates returns what change detection needs for every file of a library.
-- name: ListFileStates :many
SELECT id, item_id, relative_path, size_bytes, modified_at
FROM media_files
WHERE library_id = ?
ORDER BY relative_path;

-- name: GetLibraryKind :one
SELECT kind
FROM libraries
WHERE id = ?;

-- GetFileOwner returns the item and library a stored file belongs to.
-- name: GetFileOwner :one
SELECT item_id, library_id
FROM media_files
WHERE id = ?;

-- name: GetItemLibrary :one
SELECT library_id
FROM items
WHERE id = ?;

-- name: InsertItem :exec
INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- UpsertMediaFile writes a probe result. item_id and library_id are never changed by the
-- update: the repository has checked that a stored file keeps its item and library.
-- name: UpsertMediaFile :exec
INSERT INTO media_files (id, item_id, library_id, relative_path, size_bytes, modified_at,
                         container, format_name, duration_ms, bit_rate, probed_at, probe_error)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE
SET relative_path = excluded.relative_path,
    size_bytes    = excluded.size_bytes,
    modified_at   = excluded.modified_at,
    container     = excluded.container,
    format_name   = excluded.format_name,
    duration_ms   = excluded.duration_ms,
    bit_rate      = excluded.bit_rate,
    probed_at     = excluded.probed_at,
    probe_error   = excluded.probe_error;

-- name: DeleteFileStreams :exec
DELETE FROM media_streams
WHERE media_file_id = ?;

-- name: InsertStream :exec
INSERT INTO media_streams (media_file_id, stream_index, kind, codec, codec_tag, profile, level,
                           pixel_format, bit_depth, width, height, frame_rate, color_primaries,
                           color_transfer, color_space, hdr, channels, channel_layout,
                           sample_rate, language, title, is_default, is_forced,
                           is_attached_picture, bit_rate)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- DeleteLibraryFiles deletes the listed files of one library (their streams by cascade) and
-- returns the items they belonged to. IDs of other libraries are left alone.
-- name: DeleteLibraryFiles :many
DELETE FROM media_files
WHERE library_id = sqlc.arg(library_id) AND id IN (sqlc.slice(file_ids))
RETURNING item_id;

-- DeleteItemsWithoutFiles removes the listed items once their last file is gone. Only items
-- that lost a file are candidates, so items that never own files (the show and season
-- containers of milestone 3) survive.
-- name: DeleteItemsWithoutFiles :exec
DELETE FROM items
WHERE items.id IN (sqlc.slice(item_ids))
  AND NOT EXISTS (SELECT 1 FROM media_files WHERE media_files.item_id = items.id);

-- name: LibraryExists :one
SELECT EXISTS (SELECT 1 FROM libraries WHERE id = ?) AS found;

-- ListItemPage returns the items after the key (after_sort_title, after_id) in keyset order
-- with each item's primary file: the first of its files by relative path. A key of two
-- empty strings lies before every item, because IDs are never empty. The two conditions on
-- sort_title let SQLite scan items_library_sort_title as a range instead of filtering the
-- whole library.
-- name: ListItemPage :many
SELECT sqlc.embed(items), sqlc.embed(media_files)
FROM items
JOIN media_files ON media_files.id = (
    SELECT candidates.id
    FROM media_files AS candidates
    WHERE candidates.item_id = items.id
    ORDER BY candidates.relative_path, candidates.id
    LIMIT 1)
WHERE items.library_id = sqlc.arg(library_id)
  AND items.sort_title >= sqlc.arg(after_sort_title)
  AND (items.sort_title > sqlc.arg(after_sort_title) OR items.id > sqlc.arg(after_id))
ORDER BY items.sort_title, items.id
LIMIT sqlc.arg(row_limit);

-- name: ListStreamsOfFiles :many
SELECT *
FROM media_streams
WHERE media_file_id IN (sqlc.slice(file_ids))
ORDER BY media_file_id, stream_index;

-- name: GetItem :one
SELECT *
FROM items
WHERE id = ?;

-- ListItemFiles returns an item's files, the primary file (first by relative path) first.
-- name: ListItemFiles :many
SELECT *
FROM media_files
WHERE item_id = ?
ORDER BY relative_path, id;

-- name: GetFileWithRoot :one
SELECT sqlc.embed(media_files), libraries.root_path
FROM media_files
JOIN libraries ON libraries.id = media_files.library_id
WHERE media_files.id = ?;
