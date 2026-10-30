-- ListLibraries orders by name for display: COLLATE NOCASE keeps "anime" next to "Anime"
-- instead of behind every capitalised name, and the id makes the order total.
-- name: ListLibraries :many
SELECT libraries.id, libraries.name, libraries.kind, libraries.root_path,
       COUNT(items.id) AS item_count
FROM libraries
LEFT JOIN items ON items.library_id = libraries.id
GROUP BY libraries.id
ORDER BY libraries.name COLLATE NOCASE, libraries.name, libraries.id;

-- name: GetLibrary :one
SELECT libraries.id, libraries.name, libraries.kind, libraries.root_path,
       COUNT(items.id) AS item_count
FROM libraries
LEFT JOIN items ON items.library_id = libraries.id
WHERE libraries.id = ?
GROUP BY libraries.id;

-- name: InsertLibrary :exec
INSERT INTO libraries (id, name, kind, root_path, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?);

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
