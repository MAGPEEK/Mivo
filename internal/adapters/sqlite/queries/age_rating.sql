-- Age buckets of content ratings (ADR 0032 section 7, M7-11). The buckets come from the Go
-- parser, so the adapter writes them; see migration 00014.

-- SetItemAgeRating stores the bucket of an item's own rating and the text it came from.
-- name: SetItemAgeRating :exec
UPDATE items
SET age_rating        = ?,
    age_rating_source = ?
WHERE id = ?;

-- ListOutdatedAgeRatings computes, for an item and its whole subtree, the effective bucket
-- (the own bucket, else the parent's effective one) and the ancestor visibility of migration
-- 00016: the highest own bucket of the chain (NULL when neither the item nor an ancestor is
-- rated) and whether the chain is unrated (the top ancestor has no own rating). It
-- returns the rows whose stored values differ. sqlc cannot resolve a CTE in an UPDATE, so the
-- adapter writes them with SetEffectiveAgeRating.
-- name: ListOutdatedAgeRatings :many
WITH RECURSIVE subtree AS (
    SELECT item.id AS id,
           coalesce(item.age_rating, parent.effective_age_rating) AS effective,
           CASE WHEN item.age_rating IS NULL OR parent.visibility_age_rating > item.age_rating
                THEN parent.visibility_age_rating ELSE item.age_rating END AS visibility,
           coalesce(parent.visibility_unrated, item.age_rating IS NULL) AS unrated,
           item.effective_age_rating AS previous_effective,
           item.visibility_age_rating AS previous_visibility,
           item.visibility_unrated AS previous_unrated
    FROM items AS item
    LEFT JOIN items AS parent ON parent.id = item.parent_id
    WHERE item.id = ?
    UNION ALL
    SELECT child.id,
           coalesce(child.age_rating, subtree.effective),
           CASE WHEN child.age_rating IS NULL OR subtree.visibility > child.age_rating
                THEN subtree.visibility ELSE child.age_rating END,
           subtree.unrated,
           child.effective_age_rating, child.visibility_age_rating, child.visibility_unrated
    FROM items AS child
    JOIN subtree ON child.parent_id = subtree.id
)
SELECT subtree.id, subtree.effective, subtree.visibility, subtree.unrated
FROM subtree
WHERE subtree.previous_effective IS NOT subtree.effective
   OR subtree.previous_visibility IS NOT subtree.visibility
   OR subtree.previous_unrated IS NOT subtree.unrated;

-- name: SetEffectiveAgeRating :exec
UPDATE items
SET effective_age_rating  = ?,
    visibility_age_rating = ?,
    visibility_unrated    = ?
WHERE id = ?;

-- ListStaleAgeRatings returns items whose buckets were not computed from their current
-- content_rating. The WHERE clause matches the partial index items_age_rating_stale.
-- name: ListStaleAgeRatings :many
SELECT id, content_rating
FROM items
WHERE age_rating_source IS NOT content_rating
ORDER BY id
LIMIT ?;
