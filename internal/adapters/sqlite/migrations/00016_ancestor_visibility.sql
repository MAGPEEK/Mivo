-- Ancestor visibility of content limits (ADR 0032 section 7, amendment 2026-11-11, task
-- M7-13). An item is visible to a restricted viewer only if it and every ancestor pass the
-- content limit, each by its own effective bucket. Two stored values per item let one
-- predicate decide that without walking the ancestor chain in every query:
--
-- visibility_age_rating is the highest own bucket (age_rating) of the item and its ancestors;
-- NULL when none of them is rated. It equals the highest effective bucket of the chain, so
-- "every member passes the maximum age" is "this value is NULL or up to the maximum".
-- visibility_unrated is 1 when a member of the chain has no effective bucket, which is exactly
-- when the top ancestor has no own rating (everything below a rated item inherits at least
-- its bucket). "Hide unrated" hides the item then. The default 1 keeps a row that was never
-- computed hidden from a viewer who hides unrated titles.
--
-- The adapter maintains both where it propagates effective_age_rating (same transaction, same
-- subtree walk). Every row is marked stale here (age_rating_source NULL), so the adapter's
-- backfill computes them for the existing rows.
--
-- The content-limit predicate reads only these two columns from now on, so their index
-- replaces items_effective_age_rating.

-- +goose Up
ALTER TABLE items ADD COLUMN visibility_age_rating INTEGER CHECK (visibility_age_rating IN (0, 6, 12, 16, 18));
ALTER TABLE items ADD COLUMN visibility_unrated INTEGER NOT NULL DEFAULT 1 CHECK (visibility_unrated IN (0, 1));

DROP INDEX items_effective_age_rating;
CREATE INDEX items_visibility ON items (library_id, visibility_age_rating, visibility_unrated);

UPDATE items SET age_rating_source = NULL WHERE age_rating_source IS NOT NULL;

DROP TRIGGER items_age_rating_insert;
-- +goose StatementBegin
CREATE TRIGGER items_age_rating_insert AFTER INSERT ON items
WHEN new.age_rating_source IS NULL AND new.content_rating = ''
BEGIN
    UPDATE items
    SET age_rating_source     = '',
        effective_age_rating  = (SELECT parent.effective_age_rating FROM items AS parent
                                 WHERE parent.id = new.parent_id),
        visibility_age_rating = (SELECT parent.visibility_age_rating FROM items AS parent
                                 WHERE parent.id = new.parent_id),
        visibility_unrated    = coalesce((SELECT parent.visibility_unrated FROM items AS parent
                                          WHERE parent.id = new.parent_id), 1)
    WHERE id = new.id;
END;
-- +goose StatementEnd

-- No writer moves an item to another parent today. Should one do so, the item is stale and the
-- backfill recomputes its subtree from the new chain.
-- +goose StatementBegin
CREATE TRIGGER items_age_rating_reparent AFTER UPDATE OF parent_id ON items
WHEN new.parent_id IS NOT old.parent_id
BEGIN
    UPDATE items SET age_rating_source = NULL WHERE id = new.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER items_age_rating_reparent;
DROP TRIGGER items_age_rating_insert;
DROP INDEX items_visibility;
ALTER TABLE items DROP COLUMN visibility_unrated;
ALTER TABLE items DROP COLUMN visibility_age_rating;
CREATE INDEX items_effective_age_rating ON items (library_id, effective_age_rating);
-- +goose StatementBegin
CREATE TRIGGER items_age_rating_insert AFTER INSERT ON items
WHEN new.age_rating_source IS NULL AND new.content_rating = ''
BEGIN
    UPDATE items
    SET age_rating_source    = '',
        effective_age_rating = (SELECT parent.effective_age_rating FROM items AS parent
                                WHERE parent.id = new.parent_id)
    WHERE id = new.id;
END;
-- +goose StatementEnd
