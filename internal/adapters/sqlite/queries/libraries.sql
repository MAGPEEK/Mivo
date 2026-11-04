-- Folder key and import ledger queries shared by the re-keying migration fix-up and the
-- library store (library_manage.sql holds the library CRUD).

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
