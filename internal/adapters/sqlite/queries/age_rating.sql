-- Age buckets of content ratings (ADR 0032 section 7, M7-11). The buckets come from the Go
-- parser, so the adapter writes them; see migration 00014.

-- SetItemAgeRating stores the bucket of an item's own rating and the text it came from.
-- name: SetItemAgeRating :exec
UPDATE items
SET age_rating        = ?,
    age_rating_source = ?
WHERE id = ?;

-- ListOutdatedEffectiveAgeRatings computes the effective bucket of an item and its whole
-- subtree (the own bucket, else the parent's effective one) and returns the rows whose stored
-- value differs. sqlc cannot resolve a CTE in an UPDATE, so the adapter writes them with
-- SetEffectiveAgeRating.
-- name: ListOutdatedEffectiveAgeRatings :many
WITH RECURSIVE subtree AS (
    SELECT item.id AS id, coalesce(item.age_rating, parent.effective_age_rating) AS effective,
           item.effective_age_rating AS previous
    FROM items AS item
    LEFT JOIN items AS parent ON parent.id = item.parent_id
    WHERE item.id = ?
    UNION ALL
    SELECT child.id, coalesce(child.age_rating, subtree.effective), child.effective_age_rating
    FROM items AS child
    JOIN subtree ON child.parent_id = subtree.id
)
SELECT subtree.id, subtree.effective
FROM subtree
WHERE subtree.previous IS NOT subtree.effective;

-- name: SetEffectiveAgeRating :exec
UPDATE items
SET effective_age_rating = ?
WHERE id = ?;

-- ListStaleAgeRatings returns items whose buckets were not computed from their current
-- content_rating. The WHERE clause matches the partial index items_age_rating_stale.
-- name: ListStaleAgeRatings :many
SELECT id, content_rating
FROM items
WHERE age_rating_source IS NOT content_rating
ORDER BY id
LIMIT ?;
