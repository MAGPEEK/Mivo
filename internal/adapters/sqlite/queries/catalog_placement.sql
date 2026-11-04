-- FindChainItem returns the item of a placement level among its siblings. The
-- parent is '' for a top-level item, matching the expression of the items_match_key index.
-- name: FindChainItem :one
SELECT id
FROM items
WHERE library_id = ?
  AND ifnull(parent_id, '') = ?
  AND kind = ?
  AND match_key = ?;

-- InsertChainItem creates one item of a placement chain; the metadata columns keep their
-- defaults until the metadata writer fills them.
-- name: InsertChainItem :exec
INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, year, number,
                   number_end, absolute_number, extra_type, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- UpsertPlacedFile writes a probe result together with the file's placement. library_id and
-- root_id are never changed by the update: the repository has checked that a stored file
-- keeps its library. A new file goes into the first folder of its library (single-folder
-- behaviour until M3-19h).
-- name: UpsertPlacedFile :exec
INSERT INTO media_files (id, item_id, library_id, root_id, relative_path, size_bytes, modified_at,
                         container, format_name, duration_ms, bit_rate, probed_at, probe_error,
                         version_label, part_number)
VALUES (sqlc.arg(id), sqlc.arg(item_id), sqlc.arg(library_id),
        (SELECT library_roots.id FROM library_roots
         WHERE library_roots.library_id = sqlc.arg(library_id) AND library_roots.position = 0),
        sqlc.arg(relative_path), sqlc.arg(size_bytes), sqlc.arg(modified_at), sqlc.arg(container),
        sqlc.arg(format_name), sqlc.arg(duration_ms), sqlc.arg(bit_rate), sqlc.arg(probed_at),
        sqlc.arg(probe_error), sqlc.arg(version_label), sqlc.arg(part_number))
ON CONFLICT (id) DO UPDATE
SET item_id       = excluded.item_id,
    relative_path = excluded.relative_path,
    size_bytes    = excluded.size_bytes,
    modified_at   = excluded.modified_at,
    container     = excluded.container,
    format_name   = excluded.format_name,
    duration_ms   = excluded.duration_ms,
    bit_rate      = excluded.bit_rate,
    probed_at     = excluded.probed_at,
    probe_error   = excluded.probe_error,
    version_label = excluded.version_label,
    part_number   = excluded.part_number;

-- name: UpdateFilePlacement :execrows
UPDATE media_files
SET item_id = ?, version_label = ?, part_number = ?
WHERE id = ? AND library_id = ?;

-- DeleteItemIfUnused removes an item that owns no file and has no child except extras (which
-- go with it by cascade) and returns its parent, so the caller can check the parent next.
-- name: DeleteItemIfUnused :one
DELETE FROM items
WHERE items.id = ?
  AND NOT EXISTS (SELECT 1 FROM media_files WHERE media_files.item_id = items.id)
  AND NOT EXISTS (SELECT 1 FROM items AS child WHERE child.parent_id = items.id AND child.kind <> 'extra')
RETURNING parent_id;

-- name: ListPlacedFiles :many
SELECT id, item_id, relative_path, size_bytes, modified_at, version_label, part_number
FROM media_files
WHERE library_id = ?
ORDER BY relative_path;

-- name: ListLibraryChainItems :many
SELECT id, parent_id, kind, match_key
FROM items
WHERE library_id = ?;

-- name: ListSidecarStates :many
SELECT relative_path, kind, size_bytes, modified_at
FROM library_sidecars
WHERE library_id = ?
ORDER BY relative_path;

-- name: UpsertSidecarState :exec
INSERT INTO library_sidecars (library_id, root_id, relative_path, kind, size_bytes, modified_at)
VALUES (sqlc.arg(library_id),
        (SELECT library_roots.id FROM library_roots
         WHERE library_roots.library_id = sqlc.arg(library_id) AND library_roots.position = 0),
        sqlc.arg(relative_path), sqlc.arg(kind), sqlc.arg(size_bytes), sqlc.arg(modified_at))
ON CONFLICT (root_id, relative_path) DO UPDATE
SET kind        = excluded.kind,
    size_bytes  = excluded.size_bytes,
    modified_at = excluded.modified_at;

-- name: DeleteSidecarState :exec
DELETE FROM library_sidecars
WHERE library_id = ? AND relative_path = ?;
