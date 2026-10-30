-- The server identity is created once on first start and never changes (ADR 0004 conventions:
-- timestamps are INTEGER Unix milliseconds in UTC, IDs are text UUIDs). The CHECK keeps the
-- table at a single row, so a second identity can never be inserted next to the first one.

-- +goose Up
CREATE TABLE server_identity (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    server_id  TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

-- +goose Down
DROP TABLE server_identity;
