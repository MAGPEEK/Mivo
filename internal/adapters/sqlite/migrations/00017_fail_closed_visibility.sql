-- Fail-closed state of the ancestor visibility (ADR 0032 section 7, task M7-13b). Migration
-- 00016 left a row whose visibility was not computed yet at (NULL, 1): hidden from a viewer
-- who hides unrated titles, but shown to one with only a maximum age, because a NULL bucket
-- means "nothing in the chain is rated". An FSK 18 film was therefore visible to a "up to 6"
-- profile until the adapter's backfill had run.
--
-- From now on the uncomputed state is (18, 1), the strictest pair: it passes no maximum age
-- below 18 and no "hide unrated". It is stored
--   * for every stale row (age_rating_source IS NOT content_rating), here and by the triggers;
--   * for an insert the inheriting trigger of 00016 does not handle (a row stored with a
--     rating, or with a source that does not match it);
--   * for a row whose content_rating changes without its source (a writer that bypasses the
--     adapter); the adapter recomputes the pair in the same transaction, so its own writes
--     never show this state;
--   * for a row moved to another parent, together with the stale mark of 00016.
-- The backfill and the adapter's propagation replace the pair with the computed one. A child
-- computed below a stale parent inherits (18, 1) and is corrected when the parent is
-- recomputed, so nothing is visible early whatever the order of the rows.
--
-- A writer that moves an item must still propagate the visibility to the subtree in the same
-- transaction (propagateEffectiveAgeRating): the trigger closes the moved row only.

-- +goose Up
UPDATE items
SET visibility_age_rating = 18,
    visibility_unrated    = 1
WHERE age_rating_source IS NOT content_rating;

-- +goose StatementBegin
CREATE TRIGGER items_visibility_insert_stale AFTER INSERT ON items
WHEN new.age_rating_source IS NOT new.content_rating
     AND NOT (new.age_rating_source IS NULL AND new.content_rating = '')
BEGIN
    UPDATE items SET visibility_age_rating = 18, visibility_unrated = 1 WHERE id = new.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER items_visibility_rating_stale AFTER UPDATE OF content_rating ON items
WHEN new.age_rating_source IS NOT new.content_rating
BEGIN
    UPDATE items SET visibility_age_rating = 18, visibility_unrated = 1 WHERE id = new.id;
END;
-- +goose StatementEnd

DROP TRIGGER items_age_rating_reparent;
-- +goose StatementBegin
CREATE TRIGGER items_age_rating_reparent AFTER UPDATE OF parent_id ON items
WHEN new.parent_id IS NOT old.parent_id
BEGIN
    UPDATE items
    SET age_rating_source     = NULL,
        visibility_age_rating = 18,
        visibility_unrated    = 1
    WHERE id = new.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER items_age_rating_reparent;
-- +goose StatementBegin
CREATE TRIGGER items_age_rating_reparent AFTER UPDATE OF parent_id ON items
WHEN new.parent_id IS NOT old.parent_id
BEGIN
    UPDATE items SET age_rating_source = NULL WHERE id = new.id;
END;
-- +goose StatementEnd
DROP TRIGGER items_visibility_rating_stale;
DROP TRIGGER items_visibility_insert_stale;
UPDATE items
SET visibility_age_rating = NULL,
    visibility_unrated    = 1
WHERE age_rating_source IS NOT content_rating;
