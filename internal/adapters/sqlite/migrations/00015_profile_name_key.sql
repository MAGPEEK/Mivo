-- Unique profile names per account (ADR 0032 section 1, task M7-41). Two profiles of one
-- account never carry the same name, compared case-insensitively like account names; the
-- index also refuses the second of two racing requests.
--
-- name_key is account.NameKey(name). SQLite cannot case-fold Unicode, so the key comes from
-- Go, as for users.name_key. Rows written before this migration therefore keep a NULL key:
-- SQLite treats NULLs as distinct in a unique index, so the migration cannot fail on existing
-- data, and AccountStore.BackfillProfileNameKeys fills the keys at start-up. Where two such
-- rows of one account collide, the older one gets the key and the newer one keeps NULL (and
-- its name) until it is renamed.

-- +goose Up
ALTER TABLE profiles ADD COLUMN name_key TEXT CHECK (name_key IS NULL OR name_key <> '');

CREATE UNIQUE INDEX profiles_unique_name ON profiles (user_id, name_key);

-- +goose Down
DROP INDEX profiles_unique_name;
ALTER TABLE profiles DROP COLUMN name_key;
