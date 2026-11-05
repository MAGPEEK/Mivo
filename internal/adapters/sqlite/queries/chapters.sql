-- Chapter writer and reader (M3-14d). Chapters are stored per media file with offsets
-- relative to their file; the reader turns them into work-relative offsets.

-- name: MediaFileExists :one
SELECT EXISTS (SELECT 1 FROM media_files WHERE id = ?) AS found;

-- name: DeleteFileChapters :exec
DELETE FROM media_chapters
WHERE media_file_id = ?;

-- name: InsertFileChapter :exec
INSERT INTO media_chapters (media_file_id, ordinal, title, start_ms, end_ms)
VALUES (?, ?, ?, ?, ?);

-- ListItemChapters returns one row per chapter of an item's files in playback order: the
-- files in primary-file order (as ListItemFiles), each file's chapters by ordinal. A file
-- without chapters still yields one row with NULL chapter columns, because its duration
-- shifts the chapters of the following files; an item without files yields one row with
-- NULL file columns, and an unknown item yields none.
-- name: ListItemChapters :many
SELECT items.id AS item_id,
       media_files.id AS file_id,
       media_files.relative_path,
       media_files.duration_ms,
       media_chapters.title,
       media_chapters.start_ms,
       media_chapters.end_ms
FROM items
LEFT JOIN media_files ON media_files.item_id = items.id
LEFT JOIN library_roots ON library_roots.id = media_files.root_id
LEFT JOIN media_chapters ON media_chapters.media_file_id = media_files.id
WHERE items.id = sqlc.arg(item_id)
ORDER BY media_files.part_number, library_roots.position, media_files.relative_path, media_files.id,
         media_chapters.ordinal;

-- ListItemPartDurations returns the summed duration of the files of each listed item that
-- has files: the length of an audiobook over all its parts (task M3-14g).
-- name: ListItemPartDurations :many
SELECT item_id, CAST(SUM(duration_ms) AS INTEGER) AS duration_ms
FROM media_files
WHERE item_id IN (sqlc.slice(item_ids))
GROUP BY item_id;
