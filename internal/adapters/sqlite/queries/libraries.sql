-- ListLibraries orders by name for display: COLLATE NOCASE keeps "anime" next to "Anime"
-- instead of behind every capitalised name, and the id makes the order total.
-- name: ListLibraries :many
SELECT libraries.id, libraries.name, libraries.kind, library_roots.path AS root_path,
       COUNT(items.id) AS item_count
FROM libraries
JOIN library_roots ON library_roots.library_id = libraries.id AND library_roots.position = 0
LEFT JOIN items ON items.library_id = libraries.id
GROUP BY libraries.id
ORDER BY libraries.name COLLATE NOCASE, libraries.name, libraries.id;

-- name: GetLibrary :one
SELECT libraries.id, libraries.name, libraries.kind, library_roots.path AS root_path,
       COUNT(items.id) AS item_count
FROM libraries
JOIN library_roots ON library_roots.library_id = libraries.id AND library_roots.position = 0
LEFT JOIN items ON items.library_id = libraries.id
WHERE libraries.id = ?
GROUP BY libraries.id;

-- InsertLibrary stores a library of the configuration import (imported_from_config = 1);
-- its folder follows with InsertLibraryRoot.
-- name: InsertLibrary :exec
INSERT INTO libraries (id, name, kind, imported_from_config, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?);

-- name: InsertLibraryRoot :exec
INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
VALUES (?, ?, ?, ?, 0, ?);

-- RecordLibraryImport remembers a configuration root; an existing entry stays as it is.
-- name: RecordLibraryImport :exec
INSERT INTO library_imports (path_key, path, imported_at)
VALUES (?, ?, ?)
ON CONFLICT (path_key) DO NOTHING;

-- name: ListLibraryRoots :many
SELECT id, path, path_key
FROM library_roots
ORDER BY id;

-- name: UpdateLibraryRootKey :exec
UPDATE library_roots
SET path_key = ?
WHERE id = ?;

-- name: ListLibraryImports :many
SELECT path_key, path
FROM library_imports
ORDER BY path_key;

-- name: DeleteLibraryImport :exec
DELETE FROM library_imports
WHERE path_key = ?;

-- name: UpdateLibraryImportKey :exec
UPDATE library_imports
SET path_key = sqlc.arg(new_key)
WHERE path_key = sqlc.arg(old_key);

-- name: UpdateLibrary :exec
UPDATE libraries
SET name = ?, kind = ?, updated_at = ?
WHERE id = ?;

-- UpdateLibraryItemKinds keeps the items of a library whose kind changed consistent: in
-- milestone 2 every item's kind follows from its library's kind (library.ItemKindForLibrary).
-- name: UpdateLibraryItemKinds :exec
UPDATE items
SET kind = sqlc.arg(kind), updated_at = sqlc.arg(updated_at)
WHERE library_id = sqlc.arg(library_id) AND kind <> sqlc.arg(kind);

-- DeleteLibrary removes the library's items, files and streams by cascade.
-- name: DeleteLibrary :exec
DELETE FROM libraries
WHERE id = ?;
