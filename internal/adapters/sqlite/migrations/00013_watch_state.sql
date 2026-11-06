-- Per-profile watch state (ADR 0032 section 8; task M7-10). Additive: no existing table
-- changes. ADR 0004 conventions apply: timestamps are INTEGER Unix milliseconds in UTC, the
-- table is STRICT.
--
-- item_id has deliberately no foreign key to items: a rescan, a folder change, an item prune
-- or a library removal must never delete what a profile watched (user data). Rows of items
-- that no longer exist are ignored by the joins of the row queries; attach_key (the item's
-- provider reference at write time, '' when unmatched) keeps what a later re-attach step
-- needs. Deleting a profile removes its rows by cascade.
--
-- position_seconds 0 means no resume position; last_played_at NULL means never played.

-- +goose Up
CREATE TABLE watch_state (
    profile_id           TEXT    NOT NULL REFERENCES profiles (id) ON DELETE CASCADE,
    item_id              TEXT    NOT NULL CHECK (item_id <> ''),
    position_seconds     REAL    NOT NULL DEFAULT 0 CHECK (position_seconds >= 0),
    watched              INTEGER NOT NULL DEFAULT 0 CHECK (watched IN (0, 1)),
    play_count           INTEGER NOT NULL DEFAULT 0 CHECK (play_count >= 0),
    last_played_at       INTEGER,
    hidden_from_continue INTEGER NOT NULL DEFAULT 0 CHECK (hidden_from_continue IN (0, 1)),
    attach_key           TEXT    NOT NULL DEFAULT '',
    updated_at           INTEGER NOT NULL,
    PRIMARY KEY (profile_id, item_id)
) STRICT, WITHOUT ROWID;

-- Continue watching and Up next read a profile's recently played rows newest first.
CREATE INDEX watch_state_recent ON watch_state (profile_id, last_played_at);

-- +goose Down
DROP TABLE watch_state;
