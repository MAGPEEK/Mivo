-- Library management queries (ADR 0028): libraries with several folders, options and the
-- metadata language.

-- name: InsertManagedLibrary :exec
INSERT INTO libraries (id, name, kind, metadata_language, scan_extras, read_nfo, local_artwork,
                       imported_from_config, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: InsertManagedFolder :exec
INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
VALUES (?, ?, ?, ?, ?, ?);

-- UpdateManagedLibrary leaves the kind and the import flag alone; :execrows tells an unknown
-- id from a changed row.
-- name: UpdateManagedLibrary :execrows
UPDATE libraries
SET name = ?, metadata_language = ?, scan_extras = ?, read_nfo = ?, local_artwork = ?, updated_at = ?
WHERE id = ?;

-- DeleteManagedLibrary removes the catalog by cascade.
-- name: DeleteManagedLibrary :execrows
DELETE FROM libraries
WHERE id = ?;

-- name: ListManagedLibraries :many
SELECT id, name, kind, metadata_language, scan_extras, read_nfo, local_artwork, imported_from_config
FROM libraries
ORDER BY name COLLATE NOCASE, name, id;

-- name: GetManagedLibrary :one
SELECT id, name, kind, metadata_language, scan_extras, read_nfo, local_artwork, imported_from_config
FROM libraries
WHERE id = ?;

-- name: ListManagedFolders :many
SELECT id, library_id, path
FROM library_roots
ORDER BY library_id, position;

-- name: ListManagedFoldersOfLibrary :many
SELECT id, library_id, path
FROM library_roots
WHERE library_id = ?
ORDER BY position;

-- DeleteManagedFolder removes the folder's files, sidecars and local artwork by cascade.
-- name: DeleteManagedFolder :exec
DELETE FROM library_roots
WHERE id = ? AND library_id = ?;

-- ShiftManagedFolderPositions moves every folder of a library out of the way, so the final
-- positions cannot collide with the unique (library_id, position) while they are renumbered.
-- name: ShiftManagedFolderPositions :exec
UPDATE library_roots
SET position = position + sqlc.arg(shift)
WHERE library_id = sqlc.arg(library_id);

-- name: SetManagedFolderPosition :exec
UPDATE library_roots
SET position = ?
WHERE id = ? AND library_id = ?;

-- CountTopLevelItems counts what listLibraryItems returns per library (movies and shows, no
-- seasons, episodes or extras); it matches the items_top_level partial index. Libraries
-- without items have no row.
-- name: CountTopLevelItems :many
SELECT library_id, COUNT(*) AS item_count
FROM items
WHERE parent_id IS NULL AND kind <> 'extra'
GROUP BY library_id;

-- DeleteFolderEmptiedItem is DeleteItemIfUnused for items a removed folder left empty: an
-- extra that still has a file in a kept folder keeps its movie, because that file would
-- otherwise leave the library with the cascade. Returns the parent for the next check.
-- name: DeleteFolderEmptiedItem :one
DELETE FROM items
WHERE items.id = ?
  AND NOT EXISTS (SELECT 1 FROM media_files WHERE media_files.item_id = items.id)
  AND NOT EXISTS (
    SELECT 1 FROM items AS child
    WHERE child.parent_id = items.id
      AND (child.kind <> 'extra'
           OR EXISTS (SELECT 1 FROM media_files WHERE media_files.item_id = child.id)))
RETURNING parent_id;

-- ListFolderItemIDs returns the items that own a file in the folder, so deleting the folder
-- can prune the items it leaves without files.
-- name: ListFolderItemIDs :many
SELECT DISTINCT item_id
FROM media_files
WHERE root_id = ?;
