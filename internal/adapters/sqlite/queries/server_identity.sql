-- name: GetServerIdentity :one
SELECT server_id, created_at
FROM server_identity
WHERE id = 1;

-- InsertServerIdentity never overwrites: when the row already exists it affects no row, and
-- the repository reports that as a conflict.
-- name: InsertServerIdentity :execrows
INSERT INTO server_identity (id, server_id, created_at)
VALUES (1, ?, ?)
ON CONFLICT (id) DO NOTHING;
