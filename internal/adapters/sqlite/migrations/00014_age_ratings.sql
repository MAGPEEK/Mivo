-- Age buckets of content ratings (ADR 0032 section 7, task M7-11). Additive: three nullable
-- columns on items, two indexes and one trigger.
--
-- age_rating is the bucket (0, 6, 12, 16, 18) the domain parser (access.ParseRating) computes
-- from the item's own content_rating; NULL means unknown or no rating. effective_age_rating
-- is the bucket a content limit compares: the item's own bucket, else the effective bucket of
-- its parent, so an episode without a rating inherits its season's and thereby its show's.
-- age_rating_source is the content_rating text both values were computed from. A row whose
-- source differs from its content_rating is stale (every row after this migration, because
-- the source starts as NULL); the adapter's backfill recomputes exactly those rows.
--
-- The parser is Go code, so the adapter sets age_rating and age_rating_source on every write
-- of content_rating and propagates effective_age_rating to the subtree in the same
-- transaction. The trigger covers the one case without such a write: a new item stored
-- without a rating (the placement chain inserts items with an empty content_rating) inherits
-- its parent's effective bucket and is not stale.

-- +goose Up
ALTER TABLE items ADD COLUMN age_rating INTEGER CHECK (age_rating IN (0, 6, 12, 16, 18));
ALTER TABLE items ADD COLUMN effective_age_rating INTEGER CHECK (effective_age_rating IN (0, 6, 12, 16, 18));
ALTER TABLE items ADD COLUMN age_rating_source TEXT;

-- The content-limit predicate of the listing filter (M7-12): library plus effective bucket.
CREATE INDEX items_effective_age_rating ON items (library_id, effective_age_rating);
-- Holds only the stale rows, so the backfill finds nothing in an index lookup when nothing
-- changed.
CREATE INDEX items_age_rating_stale ON items (id) WHERE age_rating_source IS NOT content_rating;

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

-- +goose Down
DROP TRIGGER items_age_rating_insert;
DROP INDEX items_age_rating_stale;
DROP INDEX items_effective_age_rating;
ALTER TABLE items DROP COLUMN age_rating_source;
ALTER TABLE items DROP COLUMN effective_age_rating;
ALTER TABLE items DROP COLUMN age_rating;
